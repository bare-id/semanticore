package internal

import (
	"errors"
	"fmt"
	"log"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

type Repository struct {
	Major, Minor, Patch int
	VPrefix             string
	Latest              string
	ChangeLabel         string

	fixes       []string
	Features    []string
	other       []string
	tests       []string
	chores      []string
	ops         []string
	docs        []string
	perf        []string
	refactor    []string
	security    []string
	releaseDate time.Time
	Breaking    bool
	Details     []string

	changelog string

	unreleased          string
	unreleasedChangelog string

	changeLabels map[string]struct{}
	issueRefs    []int
	commits      []repoCommit
}

// repoCommit holds the minimal, non-personal data needed for emergency change
// detection: no author/committer names or e-mail addresses are kept.
type repoCommit struct {
	hash    string
	message string
}

// EmergencyConfig configures the emergency change detection performed by
// Repository.CollectEmergencySignals.
type EmergencyConfig struct {
	// Label is the change label to apply once an emergency signal is found.
	Label string
	// TrailerKey/TrailerValue identify the git trailer that marks a commit as
	// an emergency change, e.g. "change-type" / "emergency".
	TrailerKey, TrailerValue string
	// BranchPatterns are glob patterns (as understood by path.Match) matched
	// against a merge request's source branch, e.g. "hotfix/*".
	BranchPatterns []string
}

func ReadRepository(repo *git.Repository, createMajor bool) (*Repository, error) {
	return ReadRepositoryWithPrefix(repo, createMajor, "")
}

func ReadRepositoryWithPrefix(repo *git.Repository, createMajor bool, labelPrefix string) (*Repository, error) {
	repository := &Repository{
		VPrefix:      "v",
		changeLabels: map[string]struct{}{},
	}

	tags := make(map[string][]*plumbing.Reference)
	gittags, err := repo.Tags()
	if err != nil {
		return nil, fmt.Errorf("unable to read repo tags: %w", err)
	}

	err = gittags.ForEach(func(r *plumbing.Reference) error {
		tag, _ := repo.TagObject(r.Hash())
		if tag != nil {
			tags[tag.Target.String()] = append(tags[tag.Target.String()], r)
		} else {
			tags[r.Hash().String()] = append(tags[r.Hash().String()], r)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("unable to iterate git tags: %w", err)
	}

	glog, err := repo.Log(&git.LogOptions{
		Order: git.LogOrderCommitterTime,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to read repository log: %w", err)
	}

	vregex := regexp.MustCompile(`(v?)(\d+).(\d+).(\d+)`)
	var ancestor *object.Commit
	_ = glog.ForEach(func(c *object.Commit) error {
		if tags, ok := tags[c.Hash.String()]; ok {
			for _, tag := range tags {
				match := vregex.FindStringSubmatch(tag.Name().String())
				if match == nil {
					continue
				}
				tagMajor, _ := strconv.Atoi(match[2])
				tagMinor, _ := strconv.Atoi(match[3])
				tagPatch, _ := strconv.Atoi(match[4])
				if tagMajor > repository.Major || (tagMajor == repository.Major && tagMinor > repository.Minor) || (tagMajor == repository.Major && tagMinor == repository.Minor && tagPatch > repository.Patch) {
					repository.Major = tagMajor
					repository.Minor = tagMinor
					repository.Patch = tagPatch
					repository.VPrefix = match[1]
					ancestor = c
					return errors.New("done")
				}
			}
		}
		return nil
	})

	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("repo.Head() failed :%w", err)
	}

	headCommit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("unable to read head commit :%w", err)
	}

	var ignore []plumbing.Hash
	var seen map[plumbing.Hash]bool
	if ancestor != nil {
		ignore = append(ignore, ancestor.Hash)
		seen = map[plumbing.Hash]bool{ancestor.Hash: true}
	}

	var logs []*object.Commit
	_ = object.NewCommitIterBSF(headCommit, seen, ignore).ForEach(func(c *object.Commit) error {
		if a, _ := c.IsAncestor(ancestor); a {
			return storer.ErrStop
		}
		logs = append(logs, c)
		return nil
	})

	repository.Latest = fmt.Sprintf("%s%d.%d.%d", repository.VPrefix, repository.Major, repository.Minor, repository.Patch)
	log.Printf("[semanticore] Current version: %s", repository.Latest)

	reverst := regexp.MustCompile(`This reverts commit ([a-zA-Z0-9]+)`)
	_ = reverst

	reverted := make(map[string]struct{})
	updates := 0

	for _, commit := range logs {
		if _, ok := reverted[commit.Hash.String()]; ok {
			continue
		}
		msg := strings.TrimSpace(commit.Message)
		if labelPrefix != "" {
			for _, label := range ExtractPrefixedLabels(msg, labelPrefix) {
				repository.changeLabels[label] = struct{}{}
			}
		}
		seenIssues := map[int]struct{}{}
		for _, id := range repository.issueRefs {
			seenIssues[id] = struct{}{}
		}
		for _, id := range ExtractIssueRefs(msg) {
			if _, ok := seenIssues[id]; !ok {
				repository.issueRefs = append(repository.issueRefs, id)
				seenIssues[id] = struct{}{}
			}
		}
		if match := reverst.FindStringSubmatch(msg); match != nil {
			reverted[match[1]] = struct{}{}
			continue
		}

		if newVprefix, newMajor, newMinor, newPatch := DetectReleaseCommit(msg, len(commit.ParentHashes) > 1); newMajor+newMinor+newPatch > 0 {
			repository.Major = newMajor
			repository.Minor = newMinor
			repository.Patch = newPatch
			repository.VPrefix = newVprefix
			repository.Latest = fmt.Sprintf("%s%d.%d.%d", repository.VPrefix, repository.Major, repository.Minor, repository.Patch)
			log.Printf("[semanticore] found version %s at %s: %q", repository.Latest, commit.Hash, msg)

			repository.unreleased = commit.Hash.String()

			fi, err := commit.Files()
			if err == nil {
				_ = fi.ForEach(func(f *object.File) error {
					if strings.ToLower(f.Name) == "changelog.md" {
						c, _ := f.Contents()
						repository.unreleasedChangelog = "## Version " + strings.Split(c, "## Version ")[1]
						repository.unreleasedChangelog = strings.TrimSpace(repository.unreleasedChangelog)
					}
					return nil
				})
			}

			break
		}

		repository.commits = append(repository.commits, repoCommit{hash: commit.Hash.String(), message: msg})

		if len(commit.ParentHashes) > 1 {
			continue
		}
		if commit.Committer.When.After(repository.releaseDate) {
			repository.releaseDate = commit.Committer.When
		}
		typ, scope, msg, major := ParseCommitMessage(msg)
		repository.Breaking = repository.Breaking || major
		line := fmt.Sprintf("%s (%s)", msg, commit.Hash.String()[:8])
		if scope != "" {
			line = fmt.Sprintf("**%s:** %s (%s)", scope, msg, commit.Hash.String()[:8])
		}
		switch typ {
		case TypeFeat:
			repository.Features = append(repository.Features, line)
		case TypeFix:
			repository.fixes = append(repository.fixes, line)
		case TypeTest:
			repository.tests = append(repository.tests, line)
		case TypeChore:
			repository.chores = append(repository.chores, line)
		case TypeOps:
			repository.ops = append(repository.ops, line)
		case TypeDocs:
			repository.docs = append(repository.docs, line)
		case TypePerf:
			repository.perf = append(repository.perf, line)
		case TypeRefactor:
			repository.refactor = append(repository.refactor, line)
		case TypeSecurity:
			repository.security = append(repository.security, line)
		default:
			repository.other = append(repository.other, line)
		}
		updates++
	}

	if updates == 0 {
		return repository, nil
	}

	if repository.Breaking && createMajor {
		repository.Major++
		repository.Minor = 0
		repository.Patch = 0
	} else if len(repository.Features) > 0 {
		repository.Minor++
		repository.Patch = 0
	} else {
		repository.Patch++
	}

	repository.changelog = fmt.Sprintf("# Changelog\n\n## Version %s%d.%d.%d (%s)\n\n", repository.VPrefix, repository.Major, repository.Minor, repository.Patch, repository.releaseDate.Format("2006-01-02"))

	changelogentries := []struct {
		title  string
		logs   []string
		detail string
	}{
		{"### Features", repository.Features, "🆕 feature"},
		{"### Security Fixes", repository.security, "🚨 security"},
		{"### Fixes", repository.fixes, "👾 fix"},
		{"### Tests", repository.tests, "🛡 test"},
		{"### Refactoring", repository.refactor, "🔁 refactor"},
		{"### Ops and CI/CD", repository.ops, "🤖 devops"},
		{"### Documentation", repository.docs, "📚 doc"},
		{"### Performance", repository.perf, "⚡️ performance"},
		{"### Chores and tidying", repository.chores, "🧹 chore"},
		{"### Other", repository.other, "📝 other"},
	}

	for _, log := range changelogentries {
		if len(log.logs) < 1 {
			continue
		}
		repository.changelog += fmt.Sprintln(log.title)
		repository.changelog += fmt.Sprintln()
		for _, line := range log.logs {
			repository.changelog += fmt.Sprintln("- " + line)
		}
		repository.changelog += fmt.Sprintln()
		repository.Details = append(repository.Details, fmt.Sprintf("%d %s", len(log.logs), log.detail))
	}

	return repository, nil
}

func (repository *Repository) CollectIssuePrefixedLabels(backend Backend, prefix string) {
	for _, id := range repository.issueRefs {
		labels, err := backend.IssuePrefixedLabels(id, prefix)
		if err != nil {
			log.Printf("[semanticore] warning: could not fetch labels for issue #%d: %v", id, err)
			continue
		}
		for _, label := range labels {
			repository.changeLabels[strings.ToLower(strings.TrimSpace(label))] = struct{}{}
		}
	}
}

// AddChangeLabelCandidate registers an additional candidate label - e.g. one
// derived from breaking-change detection, or a label already present on the
// release request that must not be downgraded - for DetermineChangeLabel to
// consider.
func (repository *Repository) AddChangeLabelCandidate(label string) {
	label = strings.ToLower(strings.TrimSpace(label))
	if label == "" {
		return
	}
	repository.changeLabels[label] = struct{}{}
}

// CollectEmergencySignals inspects every commit in the release window for the
// emergency signals described in cfg: a "Change-Type: emergency" trailer on
// the commit itself, a source branch matching cfg.BranchPatterns, or the
// emergency label already set on the associated merge/pull request. Because a
// squash merge discards the trailers of its individual commits, the merge
// request's own commits are checked too when backend is available.
//
// Matching merge/pull requests are looked up once per commit (required to
// resolve which request a commit belongs to), but their own commit list is
// fetched and cached at most once per request per run.
func (repository *Repository) CollectEmergencySignals(backend Backend, cfg EmergencyConfig) {
	label := strings.ToLower(strings.TrimSpace(cfg.Label))
	if label == "" {
		return
	}

	alreadyFlagged := func() bool {
		_, ok := repository.changeLabels[label]
		return ok
	}
	flag := func(reason string) {
		if !alreadyFlagged() {
			log.Printf("[semanticore] emergency change detected: %s", reason)
		}
		repository.changeLabels[label] = struct{}{}
	}

	squashCommits := map[int][]string{}
	checkedBranchAndLabel := map[int]struct{}{}

	for _, commit := range repository.commits {
		if HasTrailer(commit.message, cfg.TrailerKey, cfg.TrailerValue) {
			flag(fmt.Sprintf("trailer found on commit %s", shortHash(commit.hash)))
		}

		if backend == nil {
			continue
		}

		mr, err := backend.CommitMergeRequest(commit.hash)
		if err != nil {
			log.Printf("[semanticore] warning: unable to resolve merge request for commit %s: %v", shortHash(commit.hash), err)
			continue
		}
		if mr == nil {
			continue
		}

		if _, done := checkedBranchAndLabel[mr.IID]; !done {
			checkedBranchAndLabel[mr.IID] = struct{}{}
			if matchesAnyGlob(mr.SourceBranch, cfg.BranchPatterns) {
				flag(fmt.Sprintf("source branch %q of !%d matches a configured emergency pattern", mr.SourceBranch, mr.IID))
			}
			for _, l := range mr.Labels {
				if strings.EqualFold(strings.TrimSpace(l), label) {
					flag(fmt.Sprintf("label already set on !%d", mr.IID))
					break
				}
			}
		}

		if alreadyFlagged() {
			continue
		}

		messages, cached := squashCommits[mr.IID]
		if !cached {
			messages, err = backend.MergeRequestCommitMessages(mr.IID)
			if err != nil {
				log.Printf("[semanticore] warning: unable to read commits of merge request !%d: %v", mr.IID, err)
				messages = nil
			}
			squashCommits[mr.IID] = messages
		}
		for _, msg := range messages {
			if HasTrailer(msg, cfg.TrailerKey, cfg.TrailerValue) {
				flag(fmt.Sprintf("trailer found on a squashed commit of !%d", mr.IID))
				break
			}
		}
	}
}

func shortHash(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

// matchesAnyGlob reports whether branch matches any of the given glob
// patterns (as understood by path.Match, e.g. "hotfix/*").
func matchesAnyGlob(branch string, patterns []string) bool {
	for _, pattern := range patterns {
		if ok, err := path.Match(pattern, branch); ok && err == nil {
			return true
		}
	}
	return false
}

// DetermineChangeLabel returns the highest-priority label from the configured priority list
// that matches any label collected from commits, issues, or the semantic map.
//
// defaultLabel is returned when nothing matches at all.
// It may be an empty string – in that case no label is returned for that case.
func (repository *Repository) DetermineChangeLabel(priority []string, semanticMap map[string]string, defaultLabel string) string {
	if len(priority) < 2 {
		return ""
	}

	candidates := map[string]struct{}{}
	for label := range repository.changeLabels {
		candidates[strings.ToLower(strings.TrimSpace(label))] = struct{}{}
	}

	for semanticType, label := range semanticMap {
		if repository.hasSemanticType(semanticType) {
			candidates[strings.ToLower(strings.TrimSpace(label))] = struct{}{}
		}
	}

	for _, label := range priority {
		normalized := strings.ToLower(strings.TrimSpace(label))
		if _, ok := candidates[normalized]; ok {
			return strings.TrimSpace(label)
		}
	}

	return defaultLabel
}

func (repository *Repository) hasSemanticType(semanticType string) bool {
	switch strings.ToLower(strings.TrimSpace(semanticType)) {
	case "feat", "feature", "features":
		return len(repository.Features) > 0
	case "fix", "bug", "bugs":
		return len(repository.fixes) > 0
	case "test", "tests":
		return len(repository.tests) > 0
	case "chore", "chores", "update", "updates":
		return len(repository.chores) > 0
	case "ops", "ci", "cd", "build":
		return len(repository.ops) > 0
	case "doc", "docs", "documentation":
		return len(repository.docs) > 0
	case "perf", "performance":
		return len(repository.perf) > 0
	case "refactor", "rework":
		return len(repository.refactor) > 0
	case "sec", "security":
		return len(repository.security) > 0
	case "other":
		return len(repository.other) > 0
	default:
		return false
	}
}

func (repository *Repository) CreateTag(backend Backend) error {
	if repository.unreleased == "" {
		log.Printf("[semanticore] no unreleased release commit found, skipping tag creation")
		return nil
	}
	if err := backend.CreateTag(repository.Latest, repository.unreleased); err != nil {
		return fmt.Errorf("unable to tag %s at %s: %w", repository.Latest, repository.unreleased, err)
	}
	return nil
}

func (repository *Repository) CreateRelease(backend Backend) error {
	if repository.unreleased == "" {
		log.Printf("[semanticore] no unreleased release commit found, skipping release creation")
		return nil
	}
	if err := backend.CreateRelease(repository.Latest, repository.unreleasedChangelog); err != nil {
		return fmt.Errorf("unable to release %s at %s: %w", repository.Latest, repository.unreleased, err)
	}
	return nil
}

func (repository *Repository) Changelog() string {
	return repository.changelog
}

func (repository *Repository) Version() string {
	return fmt.Sprintf("%s%d.%d.%d", repository.VPrefix, repository.Major, repository.Minor, repository.Patch)
}
