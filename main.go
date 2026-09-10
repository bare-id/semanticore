package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/bare-id/semanticore/internal"
	"github.com/bare-id/semanticore/internal/hook"
)

func try(err error) {
	if err != nil {
		panic(err)
	}
}

var (
	useBackend              = flag.String("backend", os.Getenv("SEMANTICORE_BACKEND"), "configure backend use either \"github\" or \"gitlab\" - we'll try to autodetect if empty")
	createMajor             = flag.Bool("major", false, "release major versions")
	createRelease           = flag.Bool("release", true, "create release alongside tags")
	releaseMode             = flag.String("release-mode", emptyFallback(os.Getenv("SEMANTICORE_RELEASE_MODE"), "both"), "which release step to perform: \"tag\" creates only the git tag, \"release\" only creates the platform release for an already-existing tag, \"both\" (default) does both immediately")
	releaseTag              = flag.String("release-tag", emptyFallback(os.Getenv("SEMANTICORE_RELEASE_TAG"), ""), "tag to publish a release for when -release-mode=release; auto-detected from CI_COMMIT_TAG or GITHUB_REF_NAME/GITHUB_REF_TYPE when empty")
	createMergeRequest      = flag.Bool("merge-request", true, "create merge release for branch")
	authorName              = flag.String("git-author-name", emptyFallback(os.Getenv("GIT_AUTHOR_NAME"), "Semanticore Bot"), "author name for the git commits, falls back to env var GIT_AUTHOR_NAME and afterwards to \"Semanticore Bot\"")
	authorEmail             = flag.String("git-author-email", emptyFallback(os.Getenv("GIT_AUTHOR_EMAIL"), "semanticore@aoe.com"), "author email for the git commits, falls back to env var GIT_AUTHOR_EMAIL and afterwards to \"semanticore@aoe.com\"")
	committerName           = flag.String("git-committer-name", emptyFallback(os.Getenv("GIT_COMMITTER_NAME"), "Semanticore Bot"), "committer name for the git commits, falls back to env var GIT_COMMITTER_NAME and afterwards to \"Semanticore Bot\"")
	committerEmail          = flag.String("git-committer-email", emptyFallback(os.Getenv("GIT_COMMITTER_EMAIL"), "semanticore@aoe.com"), "committer email for the git commits, falls back to env var GIT_COMMITTER_EMAIL and afterwards to \"semanticore@aoe.com\"")
	changelogMaxLines       = flag.Int("changelog-max-lines", 0, "trim the changelog to the last version including the maximum configured lines")
	changelogFileName       = flag.String("changelog-file-name", emptyFallback(os.Getenv("CHANGELOG_FILE_NAME"), "Changelog.md"), "filename for changelog, falls back to env var CHANGELOG_FILE_NAME and afterwards to \"Changelog.md\"")
	signKeyFilePath         = flag.String("sign-key-file", emptyFallback(os.Getenv("SEMANTICORE_SIGN_KEY_FILE"), ""), "path to GPG private key file for signing commits")
	changeLabelsEnabled     = flag.Bool("change-labels-enabled", strings.EqualFold(os.Getenv("SEMANTICORE_CHANGE_LABELS_ENABLED"), "true"), "enable change label sync for merge requests")
	changeLabels            = flag.String("change-labels", emptyFallback(os.Getenv("SEMANTICORE_CHANGE_LABELS"), ""), "CSV list of labels in priority order (highest first), e.g. change::emergency,change::major,change::normal,change::standard")
	changeLabelMap          = flag.String("change-label-map", emptyFallback(os.Getenv("SEMANTICORE_CHANGE_LABEL_MAP"), ""), "CSV list mapping semantic commit types to labels, e.g. feat=change::normal,chore=change::standard")
	changeLabelDefault      = flag.String("change-label-default", emptyFallback(os.Getenv("SEMANTICORE_CHANGE_LABEL_DEFAULT"), ""), "label to use when no commit label matches; must be in --change-labels")
	changeLabelPolicy       = flag.String("change-label-policy", emptyFallback(os.Getenv("SEMANTICORE_CHANGE_LABEL_POLICY"), "upgrade-only"), "\"upgrade-only\" (default, never downgrade a higher-ranked label) or \"overwrite\"")
	changeLabelBreaking     = flag.String("change-label-breaking", emptyFallback(os.Getenv("SEMANTICORE_CHANGE_LABEL_BREAKING"), ""), "label to add (in addition) when a breaking change is detected; must be in --change-labels, empty disables this")
	changeEmergencyEnabled  = flag.Bool("change-emergency-enabled", strings.EqualFold(os.Getenv("SEMANTICORE_CHANGE_EMERGENCY_ENABLED"), "true"), "enable emergency change detection, requires --change-labels-enabled")
	changeEmergencyLabel    = flag.String("change-emergency-label", emptyFallback(os.Getenv("SEMANTICORE_CHANGE_EMERGENCY_LABEL"), "change::emergency"), "label to apply when an emergency change is detected; must be in --change-labels")
	changeEmergencyTrailer  = flag.String("change-emergency-trailer", emptyFallback(os.Getenv("SEMANTICORE_CHANGE_EMERGENCY_TRAILER"), "Change-Type: emergency"), "git trailer, as \"Key: Value\", that marks a commit as an emergency change")
	changeEmergencyBranches = flag.String("change-emergency-branches", emptyFallback(os.Getenv("SEMANTICORE_CHANGE_EMERGENCY_BRANCHES"), "hotfix/*"), "CSV list of glob patterns matched against a merge request's source branch")
)

func main() {
	flag.Parse()

	dir := "."
	if flag.NArg() > 0 {
		dir = flag.Arg(0)
	}
	try(os.Chdir(dir))

	repo, err := git.PlainOpen(".")
	try(err)

	remote, err := repo.Remote("origin")
	try(err)
	remoteUrl, err := url.Parse(remote.Config().URLs[0])
	try(err)
	repoId := strings.TrimSuffix(strings.TrimPrefix(remoteUrl.Path, "/"), ".git")
	log.Printf("[semanticore] repository: %s at %s", repoId, remoteUrl.Host)

	var backend internal.Backend
	if os.Getenv("SEMANTICORE_TOKEN") == "" {
		log.Println("[semanticore] SEMANTICORE_TOKEN unset, no merge requests will be handled")
	} else if *useBackend == "github" || remoteUrl.Host == "github.com" {
		backend = internal.NewGithubBackend(os.Getenv("SEMANTICORE_TOKEN"), repoId)
	} else if *useBackend == "gitlab" || strings.Contains(remoteUrl.Host, "gitlab") {
		backend = internal.NewGitlabBackend(os.Getenv("SEMANTICORE_TOKEN"), remoteUrl.Host, repoId)
	}

	head, err := repo.Head()
	try(err)

	// derive label prefix from configured labels when the feature is enabled
	labelPrefix := ""
	if *changeLabelsEnabled {
		if priority, ok := parseChangeLabels(*changeLabels); ok {
			labelPrefix = commonLabelPrefix(priority)
		}
	}

	repository, err := internal.ReadRepositoryWithPrefix(repo, *createMajor, labelPrefix)
	try(err)

	if backend != nil && *createRelease {
		switch strings.ToLower(*releaseMode) {
		case "release":
			tag := *releaseTag
			if tag == "" {
				tag = detectCurrentTag()
			}
			if tag == "" {
				log.Printf("[semanticore] release-mode=release requires a tag (-release-tag, SEMANTICORE_RELEASE_TAG, or a CI tag pipeline)")
			} else if section, err := readChangelogSectionForTag(*changelogFileName, tag); err != nil {
				log.Printf("[semanticore] unable to read changelog for %s: %v", tag, err)
			} else if err := backend.CreateRelease(tag, section); err != nil {
				log.Printf("[semanticore] unable to create release: %v", err)
			}
			return
		case "tag":
			if err := repository.CreateTag(backend); err != nil {
				log.Printf("[semanticore] unable to create tag: %v", err)
			}
		default:
			if err := repository.CreateTag(backend); err != nil {
				log.Printf("[semanticore] unable to create tag: %v", err)
			} else if err := repository.CreateRelease(backend); err != nil {
				log.Printf("[semanticore] unable to create release: %v", err)
			}
		}
	}

	changelog := repository.Changelog()

	if changelog == "" {
		log.Println("no changes detected, exiting...")
		return
	}

	fmt.Println(changelog)

	if !*createMergeRequest {
		return
	}

	wt, err := repo.Worktree()
	try(err)

	filename := *changelogFileName
	files, err := wt.Filesystem.ReadDir(".")
	try(err)

	// detect case-sensitive filenames
	for _, f := range files {
		if !f.IsDir() && strings.EqualFold(f.Name(), filename) {
			filename = f.Name()
		}
	}

	cl, _ := os.ReadFile(filepath.Join(filename))

	if *changelogMaxLines > 0 {
		cl = internal.TrimChangelog(cl, *changelogMaxLines)
	}

	if strings.Contains(string(cl), "# Changelog\n\n") {
		cl = bytes.Replace(cl, []byte("# Changelog\n\n"), []byte(changelog), 1)
	} else if strings.Contains(string(cl), "# Changelog\n") {
		cl = bytes.Replace(cl, []byte("# Changelog\n"), []byte(changelog), 1)
	} else {
		cl = append([]byte(changelog), cl...)
	}
	try(os.WriteFile(filepath.Join(filename), cl, 0644))

	_, err = wt.Add(filename)
	try(err)

	hook.NpmUpdateVersionHook(wt, repository)

	signKey, err := internal.TryCreateSignKey(signKeyFilePath)
	if errors.Is(err, internal.ErrNoSigningKeyFound) {
		log.Printf("[semanticore] no signing key found, commit will not be signed")
	} else if err != nil {
		try(err)
	}

	commitOptions := &git.CommitOptions{
		Author: &object.Signature{
			Name:  *authorName,
			Email: *authorEmail,
			When:  time.Now(),
		},
		Committer: &object.Signature{
			Name:  *committerName,
			Email: *committerEmail,
			When:  time.Now(),
		},
		SignKey: signKey,
	}

	commit, err := wt.Commit(fmt.Sprintf("Release %s%d.%d.%d", repository.VPrefix, repository.Major, repository.Minor, repository.Patch), commitOptions)
	try(err)

	log.Printf("[semanticore] committed changelog: %s", commit.String())

	try(wt.Reset(&git.ResetOptions{
		Commit: head.Hash(),
		Mode:   git.HardReset,
	}))

	if backend == nil {
		log.Printf("no backend configured, keeping changes in a local commit: %s", commit.String())
		return
	}
	try(repo.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{config.RefSpec(commit.String() + ":refs/heads/semanticore/release")},
		Force:      true,
		Auth:       backend,
		Progress:   os.Stdout,
	}))

	releasetype := "patch 🩹"
	if repository.Breaking && *createMajor {
		releasetype = "major 👏"
	} else if len(repository.Features) > 0 {
		releasetype = "minor 📦"
	}
	labels := "Release 🏆," + releasetype
	if *changeLabelsEnabled {
		if priority, ok := parseChangeLabels(*changeLabels); ok {
			if backend != nil {
				repository.CollectIssuePrefixedLabels(backend, labelPrefix)
			}
			defaultLabel, defaultOk := parseChangeLabelDefault(*changeLabelDefault, priority)
			if !defaultOk {
				log.Printf("[semanticore] change labels are enabled but SEMANTICORE_CHANGE_LABEL_DEFAULT is not part of SEMANTICORE_CHANGE_LABELS, disabling feature")
			} else if semanticMap, ok := parseChangeLabelMap(*changeLabelMap, priority); ok {
				policy, policyOk := parseChangeLabelPolicy(*changeLabelPolicy)
				if !policyOk {
					log.Fatalf("[semanticore] SEMANTICORE_CHANGE_LABEL_POLICY must be \"upgrade-only\" or \"overwrite\", got %q", *changeLabelPolicy)
				}

				breakingLabel, breakingOk := parseChangeLabelDefault(*changeLabelBreaking, priority)
				if !breakingOk {
					log.Fatalf("[semanticore] SEMANTICORE_CHANGE_LABEL_BREAKING %q is not part of SEMANTICORE_CHANGE_LABELS", *changeLabelBreaking)
				}

				if *changeEmergencyEnabled {
					emergencyLabel, emergencyOk := parseChangeLabelDefault(*changeEmergencyLabel, priority)
					if !emergencyOk || emergencyLabel == "" {
						log.Fatalf("[semanticore] SEMANTICORE_CHANGE_EMERGENCY_LABEL %q is not part of SEMANTICORE_CHANGE_LABELS", *changeEmergencyLabel)
					}
					trailerKey, trailerValue, trailerOk := parseEmergencyTrailer(*changeEmergencyTrailer)
					if !trailerOk {
						log.Fatalf("[semanticore] SEMANTICORE_CHANGE_EMERGENCY_TRAILER %q must be in \"Key: Value\" form", *changeEmergencyTrailer)
					}

					repository.CollectEmergencySignals(backend, internal.EmergencyConfig{
						Label:          emergencyLabel,
						TrailerKey:     trailerKey,
						TrailerValue:   trailerValue,
						BranchPatterns: parseCSVList(*changeEmergencyBranches),
					})
				}

				if breakingLabel != "" && repository.Breaking {
					repository.AddChangeLabelCandidate(breakingLabel)
					log.Printf("[semanticore] breaking change detected in this release - please verify whether the related epic should be labeled change::major (semanticore never sets change::major itself)")
				}

				if policy == "upgrade-only" && backend != nil {
					if existing, err := backend.CurrentChangeLabels(labelPrefix); err != nil {
						log.Printf("[semanticore] warning: could not read existing release labels, downgrade protection may not apply: %v", err)
					} else {
						for _, label := range existing {
							repository.AddChangeLabelCandidate(label)
						}
					}
				}

				repository.ChangeLabel = repository.DetermineChangeLabel(priority, semanticMap, defaultLabel)
				labels += "," + repository.ChangeLabel
			} else {
				log.Printf("[semanticore] change labels are enabled but SEMANTICORE_CHANGE_LABEL_MAP is invalid, disabling feature")
			}
		} else {
			log.Printf("[semanticore] change labels are enabled but SEMANTICORE_CHANGE_LABELS is invalid, disabling feature")
		}
	}
	description := fmt.Sprintf(`# Release %s%d.%d.%d 🏆

## Summary

There are %s commits since %s.

This is a %s release.

Merge this pull request to commit the changelog and have Semanticore create a new release on the next pipeline run.

%s

---

This changelog was generated by your friendly [Semanticore Release Bot](https://github.com/bare-id/semanticore)
`, repository.VPrefix, repository.Major, repository.Minor, repository.Patch, strings.Join(repository.Details, ", "), repository.Latest, releasetype, strings.TrimSpace(changelog))

	mainBranch, err := backend.MainBranch()
	try(err)

	try(backend.MergeRequest(string(mainBranch), fmt.Sprintf("Release %s%d.%d.%d", repository.VPrefix, repository.Major, repository.Minor, repository.Patch), description, labels))
}

// detectCurrentTag returns the tag name a CI system checked out the current
// pipeline for, or "" when running outside of a tag pipeline.
func detectCurrentTag() string {
	if tag := os.Getenv("CI_COMMIT_TAG"); tag != "" {
		return tag
	}
	if os.Getenv("GITHUB_REF_TYPE") == "tag" {
		return os.Getenv("GITHUB_REF_NAME")
	}
	return ""
}

// readChangelogSectionForTag locates the changelog file (matching filename
// case-insensitively, like the release commit logic above) and returns the
// section describing tag.
func readChangelogSectionForTag(filename, tag string) (string, error) {
	files, err := os.ReadDir(".")
	if err != nil {
		return "", err
	}

	for _, f := range files {
		if !f.IsDir() && strings.EqualFold(f.Name(), filename) {
			filename = f.Name()
		}
	}

	cl, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}

	return internal.ExtractChangelogSection(cl, tag)
}

func emptyFallback(s, fallback string) string {
	if s == "" {
		return fallback
	}

	return s
}

func parseChangeLabels(raw string) ([]string, bool) {
	parts := strings.Split(raw, ",")
	if len(parts) < 2 {
		return nil, false
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		label := strings.TrimSpace(strings.ToLower(part))
		if label == "" || !strings.Contains(label, "::") {
			return nil, false
		}
		if _, ok := seen[label]; ok {
			return nil, false
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}

	return out, true
}

// commonLabelPrefix returns the longest common prefix shared by all labels,
// e.g. ["change::emergency","change::normal"] → "change::".
// Returns "" if no common prefix exists.
func commonLabelPrefix(labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	prefix := labels[0]
	for _, label := range labels[1:] {
		for !strings.HasPrefix(label, prefix) {
			if len(prefix) == 0 {
				return ""
			}
			prefix = prefix[:len(prefix)-1]
		}
	}
	return prefix
}

// parseChangeLabelDefault validates that defaultLabel is present in the
// priority list. Empty string is allowed (no default configured).
func parseChangeLabelDefault(defaultLabel string, priority []string) (string, bool) {
	allowed := map[string]struct{}{}
	for _, l := range priority {
		allowed[strings.ToLower(strings.TrimSpace(l))] = struct{}{}
	}
	dl := strings.ToLower(strings.TrimSpace(defaultLabel))
	if dl != "" {
		if _, ok := allowed[dl]; !ok {
			return "", false
		}
	}
	return dl, true
}

// parseChangeLabelPolicy validates SEMANTICORE_CHANGE_LABEL_POLICY. The only
// accepted values are "upgrade-only" and "overwrite" (case-insensitive).
func parseChangeLabelPolicy(raw string) (string, bool) {
	policy := strings.ToLower(strings.TrimSpace(raw))
	if policy != "upgrade-only" && policy != "overwrite" {
		return "", false
	}
	return policy, true
}

// parseEmergencyTrailer splits a "Key: Value" configuration string, as used
// for SEMANTICORE_CHANGE_EMERGENCY_TRAILER, into its key and value.
func parseEmergencyTrailer(raw string) (key, value string, ok bool) {
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	key = strings.TrimSpace(parts[0])
	value = strings.TrimSpace(parts[1])
	if key == "" || value == "" {
		return "", "", false
	}
	return key, value, true
}

// parseCSVList splits a comma-separated list into trimmed, non-empty entries.
func parseCSVList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseChangeLabelMap(raw string, priority []string) (map[string]string, bool) {
	allowed := map[string]struct{}{}
	for _, label := range priority {
		allowed[strings.ToLower(strings.TrimSpace(label))] = struct{}{}
	}

	out := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return out, true
	}

	for _, entry := range strings.Split(raw, ",") {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			return nil, false
		}

		semanticType := strings.ToLower(strings.TrimSpace(parts[0]))
		label := strings.ToLower(strings.TrimSpace(parts[1]))
		if semanticType == "" || label == "" {
			return nil, false
		}
		if _, ok := allowed[label]; !ok {
			return nil, false
		}
		if _, ok := out[semanticType]; ok {
			return nil, false
		}

		out[semanticType] = label
	}

	return out, true
}
