package internal

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/stretchr/testify/assert"
)

type testBackend struct {
	tag       string
	ref       string
	changelog string
}

func (*testBackend) String() string { return "testBackend" }
func (*testBackend) Name() string   { return "testBackend" }
func (b *testBackend) CreateTag(tag, ref string) error {
	b.tag = tag
	b.ref = ref
	return nil
}
func (b *testBackend) CreateRelease(tag, changelog string) error {
	b.tag = tag
	b.changelog = changelog
	return nil
}
func (*testBackend) MergeRequest(_, _, _, _ string) error                  { return nil }
func (*testBackend) CloseMergeRequest() error                              { return nil }
func (*testBackend) MainBranch() (string, error)                           { return "main", nil }
func (*testBackend) IssuePrefixedLabels(_ int, _ string) ([]string, error) { return nil, nil }
func (*testBackend) CommitMergeRequest(_ string) (*MergeRequestRef, error) { return nil, nil }
func (*testBackend) MergeRequestCommitMessages(_ int) ([]string, error)    { return nil, nil }
func (*testBackend) CurrentChangeLabels(_ string) ([]string, error)        { return nil, nil }
func (*testBackend) SetAuth(_ *http.Request)                               {}

func TestReadRepository(t *testing.T) {
	mockRepo, err := git.Init(memory.NewStorage(), memfs.New())
	assert.NoError(t, err)

	cfg, err := mockRepo.Config()
	assert.NoError(t, err)
	cfg.Author.Email = "testing@example.com"
	cfg.Author.Name = "testing"
	cfg.User.Email = "testing@example.com"
	cfg.User.Name = "testing"
	err = mockRepo.SetConfig(cfg)
	assert.NoError(t, err)

	assert.NoError(t, mockRepo.CreateBranch(&config.Branch{Name: "main"}))
	mockWt, err := mockRepo.Worktree()
	assert.NoError(t, err)

	_ = mockWt.Checkout(&git.CheckoutOptions{Branch: "main"})

	_, err = ReadRepository(mockRepo, true)
	assert.Error(t, err)

	file, err := mockWt.Filesystem.Create("test.file")
	assert.NoError(t, err)

	testCommit := func(msg string) plumbing.Hash {
		_, err := file.Write([]byte("msg"))
		assert.NoError(t, err)
		_, err = mockWt.Add("test.file")
		assert.NoError(t, err)
		hash, err := mockWt.Commit(msg, &git.CommitOptions{})
		assert.NoError(t, err)
		return hash
	}

	testCommit("test(semanticore): initial commit")

	repository, err := ReadRepository(mockRepo, true)
	assert.NoError(t, err)
	assert.Equal(t, "", repository.unreleased)
	assert.Equal(t, "", repository.unreleasedChangelog)
	assert.Len(t, repository.tests, 1)

	vhash := testCommit("ci(semanticore): initial ci")
	_, err = mockRepo.CreateTag("v0.0.1", vhash, nil)
	assert.NoError(t, err)
	repository, err = ReadRepository(mockRepo, true)
	assert.NoError(t, err)
	assert.Equal(t, "v0.0.1", repository.Latest)
	assert.Equal(t, "", repository.unreleased)

	vhash = testCommit("ci(semanticore): initial ci")
	_, err = mockRepo.CreateTag("v0.0.2", vhash, &git.CreateTagOptions{Message: "v0.0.2"})
	assert.NoError(t, err)
	repository, err = ReadRepository(mockRepo, true)
	assert.NoError(t, err)
	assert.Equal(t, "v0.0.2", repository.Latest)
	assert.Equal(t, "", repository.changelog)

	cf, err := mockWt.Filesystem.Create("Changelog.md")
	assert.NoError(t, err)
	defer func() { _ = cf.Close() }()
	_, err = cf.Write([]byte(`## Version 1.2.3 test ## Version 1.2.3 ## Version 1.2.3`))
	assert.NoError(t, err)
	_, err = mockWt.Add("Changelog.md")
	assert.NoError(t, err)
	vhash = testCommit("Release v0.0.3")
	repository, err = ReadRepository(mockRepo, true)
	assert.NoError(t, err)
	assert.Equal(t, "v0.0.3", repository.Latest)
	assert.Equal(t, vhash.String(), repository.unreleased)
	assert.Equal(t, "## Version 1.2.3 test", repository.unreleasedChangelog)
	testBackend := new(testBackend)
	assert.NoError(t, repository.CreateTag(testBackend))
	assert.Equal(t, vhash.String(), testBackend.ref)
	assert.NoError(t, repository.CreateRelease(testBackend))
	assert.Equal(t, "## Version 1.2.3 test", testBackend.changelog)

	testCommit("ci(semanticore): next ci")
	testCommit("test(semanticore): next test")
	testCommit("chore(semanticore): initial chore")
	testCommit("docs(semanticore): initial docs")
	testCommit("perf(semanticore): initial perf")
	testCommit("refactor(semanticore): initial refactor")
	testCommit("security(semanticore): initial security")
	testCommit("initial something whatever")
	testCommit("task: initial task")

	repository, err = ReadRepository(mockRepo, true)
	assert.NoError(t, err)
	assert.Len(t, repository.tests, 1)
	assert.Len(t, repository.ops, 1)
	assert.Equal(t, 0, repository.Major)
	assert.Equal(t, 0, repository.Minor)
	assert.Equal(t, 4, repository.Patch)

	testCommit("feat(semanticore): initial feature")

	repository, err = ReadRepository(mockRepo, true)
	assert.NoError(t, err)
	assert.Len(t, repository.tests, 1)
	assert.Len(t, repository.ops, 1)
	assert.Equal(t, 0, repository.Major)
	assert.Equal(t, 1, repository.Minor)
	assert.Equal(t, 0, repository.Patch)

	testCommit("feat(semanticore): second feature")

	repository, err = ReadRepository(mockRepo, true)
	assert.NoError(t, err)
	assert.Len(t, repository.tests, 1)
	assert.Len(t, repository.ops, 1)
	assert.Equal(t, 0, repository.Major)
	assert.Equal(t, 1, repository.Minor)
	assert.Equal(t, 0, repository.Patch)

	testCommit("fix(semanticore): initial fix")
	testCommit("fix(semanticore): second fix")

	testCommit("fix(semanticore)!: final fix")

	repository, err = ReadRepository(mockRepo, true)
	assert.NoError(t, err)
	assert.Len(t, repository.tests, 1)
	assert.Len(t, repository.ops, 1)
	assert.Len(t, repository.fixes, 3)
	assert.Equal(t, 1, repository.Major)
	assert.Equal(t, 0, repository.Minor)
	assert.Equal(t, 0, repository.Patch)

	repository, err = ReadRepository(mockRepo, false)
	assert.NoError(t, err)
	assert.Len(t, repository.tests, 1)
	assert.Len(t, repository.ops, 1)
	assert.Len(t, repository.fixes, 3)
	assert.Equal(t, 0, repository.Major)
	assert.Equal(t, 1, repository.Minor)
	assert.Equal(t, 0, repository.Patch)
}

func TestCollectIssuePrefixedLabels(t *testing.T) {
	priority := []string{"change::emergency", "change::major", "change::normal", "change::standard"}

	backend := &issueBackend{labels: map[int][]string{
		42:  {"change::major", "other-label"},
		123: {"change::normal"},
		99:  nil,
	}}

	repo := &Repository{
		changeLabels: map[string]struct{}{},
		issueRefs:    []int{42, 123, 99, 999},
	}
	repo.CollectIssuePrefixedLabels(backend, "change::")

	assert.Equal(t, "change::major", repo.DetermineChangeLabel(priority, nil, "change::standard"))
}

type issueBackend struct {
	labels map[int][]string
}

func (*issueBackend) String() string                                        { return "issueBackend" }
func (*issueBackend) Name() string                                          { return "issueBackend" }
func (*issueBackend) SetAuth(_ *http.Request)                               {}
func (*issueBackend) CreateTag(_, _ string) error                           { return nil }
func (*issueBackend) CreateRelease(_, _ string) error                       { return nil }
func (*issueBackend) MergeRequest(_, _, _, _ string) error                  { return nil }
func (*issueBackend) CloseMergeRequest() error                              { return nil }
func (*issueBackend) MainBranch() (string, error)                           { return "main", nil }
func (*issueBackend) CommitMergeRequest(_ string) (*MergeRequestRef, error) { return nil, nil }
func (*issueBackend) MergeRequestCommitMessages(_ int) ([]string, error)    { return nil, nil }
func (*issueBackend) CurrentChangeLabels(_ string) ([]string, error)        { return nil, nil }
func (b *issueBackend) IssuePrefixedLabels(id int, prefix string) ([]string, error) {
	labels, ok := b.labels[id]
	if !ok {
		return nil, fmt.Errorf("issue %d not found", id)
	}
	var out []string
	for _, label := range labels {
		if strings.HasPrefix(strings.ToLower(label), strings.ToLower(prefix)) {
			out = append(out, label)
		}
	}
	return out, nil
}

func TestDetermineChangeLabel(t *testing.T) {
	priority := []string{"change::emergency", "change::major", "change::normal", "change::standard"}
	dl := "change::standard"

	// explicit label match
	repo := &Repository{changeLabels: map[string]struct{}{"change::major": {}}}
	assert.Equal(t, "change::major", repo.DetermineChangeLabel(priority, nil, dl))

	// no feature mapping -> default
	repo = &Repository{Features: []string{"new feat"}, changeLabels: map[string]struct{}{}}
	assert.Equal(t, "change::standard", repo.DetermineChangeLabel(priority, nil, dl))

	// feature mapping via semantic map
	repo = &Repository{Features: []string{"new feat"}, changeLabels: map[string]struct{}{}}
	assert.Equal(t, "change::normal", repo.DetermineChangeLabel(priority, map[string]string{"feat": "change::normal"}, dl))

	// no match → default
	repo = &Repository{changeLabels: map[string]struct{}{}}
	assert.Equal(t, "change::standard", repo.DetermineChangeLabel(priority, nil, dl))

	// no default configured → empty string
	repo = &Repository{Features: []string{"x"}, changeLabels: map[string]struct{}{}}
	assert.Equal(t, "", repo.DetermineChangeLabel(priority, nil, ""))

	// semantic map match
	repo = &Repository{chores: []string{"chore message"}, changeLabels: map[string]struct{}{}}
	assert.Equal(t, "change::standard", repo.DetermineChangeLabel(priority, map[string]string{"chore": "change::standard"}, dl))

	// explicit label beats lower-priority semantic map entry
	repo = &Repository{ops: []string{"ops message"}, changeLabels: map[string]struct{}{"change::major": {}}}
	assert.Equal(t, "change::major", repo.DetermineChangeLabel(priority, map[string]string{"ops": "change::standard"}, dl))

	// 3-label variant with explicit default + map
	priority3 := []string{"change::emergency", "change::normal", "change::standard"}
	repo = &Repository{changeLabels: map[string]struct{}{"change::emergency": {}}}
	assert.Equal(t, "change::emergency", repo.DetermineChangeLabel(priority3, nil, "change::standard"))
	repo = &Repository{Features: []string{"x"}, changeLabels: map[string]struct{}{}}
	assert.Equal(t, "change::normal", repo.DetermineChangeLabel(priority3, map[string]string{"feat": "change::normal"}, "change::standard"))
	repo = &Repository{changeLabels: map[string]struct{}{}}
	assert.Equal(t, "change::standard", repo.DetermineChangeLabel(priority3, nil, "change::standard"))

	// 5-label variant with custom mapping + explicit default
	priority5 := []string{"change::emergency", "change::major", "change::normal", "change::minor", "change::standard"}
	repo = &Repository{Features: []string{"x"}, changeLabels: map[string]struct{}{}}
	assert.Equal(t, "change::minor", repo.DetermineChangeLabel(priority5, map[string]string{"feat": "change::minor"}, "change::standard"))
	repo = &Repository{changeLabels: map[string]struct{}{}}
	assert.Equal(t, "change::standard", repo.DetermineChangeLabel(priority5, nil, "change::standard"))

	// < 2 priority → disabled
	assert.Equal(t, "", repo.DetermineChangeLabel([]string{"change::major"}, nil, ""))
	assert.Equal(t, "", repo.DetermineChangeLabel(nil, nil, ""))
}

func TestAddChangeLabelCandidate(t *testing.T) {
	priority := []string{"change::emergency", "change::major", "change::normal", "change::standard"}

	// feeding in a label already present on the release request protects it
	// from being downgraded by a lower-ranked recomputed candidate (upgrade-only policy)
	repo := &Repository{Features: []string{"x"}, changeLabels: map[string]struct{}{}}
	repo.AddChangeLabelCandidate("change::emergency")
	assert.Equal(t, "change::emergency", repo.DetermineChangeLabel(priority, map[string]string{"feat": "change::normal"}, "change::standard"))

	// case-insensitive, trims whitespace
	repo = &Repository{changeLabels: map[string]struct{}{}}
	repo.AddChangeLabelCandidate("  Change::Major  ")
	assert.Equal(t, "change::major", repo.DetermineChangeLabel(priority, nil, "change::standard"))

	// empty label is a no-op
	repo = &Repository{changeLabels: map[string]struct{}{}}
	repo.AddChangeLabelCandidate("")
	assert.Equal(t, "change::standard", repo.DetermineChangeLabel(priority, nil, "change::standard"))
}

func TestMatchesAnyGlob(t *testing.T) {
	assert.True(t, matchesAnyGlob("hotfix/payment-fix", []string{"hotfix/*"}))
	assert.True(t, matchesAnyGlob("hotfix/payment-fix", []string{"release/*", "hotfix/*"}))
	assert.False(t, matchesAnyGlob("feature/payment", []string{"hotfix/*"}))
	assert.False(t, matchesAnyGlob("hotfix/nested/branch", []string{"hotfix/*"}))
	assert.False(t, matchesAnyGlob("hotfix/payment-fix", nil))
}

type emergencyBackend struct {
	mrsBySha     map[string]*MergeRequestRef
	commitsByIID map[int][]string
	commitCalls  map[int]int
}

func (*emergencyBackend) String() string                                        { return "emergencyBackend" }
func (*emergencyBackend) Name() string                                          { return "emergencyBackend" }
func (*emergencyBackend) SetAuth(_ *http.Request)                               {}
func (*emergencyBackend) CreateTag(_, _ string) error                           { return nil }
func (*emergencyBackend) CreateRelease(_, _ string) error                       { return nil }
func (*emergencyBackend) MergeRequest(_, _, _, _ string) error                  { return nil }
func (*emergencyBackend) CloseMergeRequest() error                              { return nil }
func (*emergencyBackend) MainBranch() (string, error)                           { return "main", nil }
func (*emergencyBackend) IssuePrefixedLabels(_ int, _ string) ([]string, error) { return nil, nil }
func (*emergencyBackend) CurrentChangeLabels(_ string) ([]string, error)        { return nil, nil }
func (b *emergencyBackend) CommitMergeRequest(sha string) (*MergeRequestRef, error) {
	return b.mrsBySha[sha], nil
}
func (b *emergencyBackend) MergeRequestCommitMessages(iid int) ([]string, error) {
	if b.commitCalls == nil {
		b.commitCalls = map[int]int{}
	}
	b.commitCalls[iid]++
	return b.commitsByIID[iid], nil
}

func emergencyCfg() EmergencyConfig {
	return EmergencyConfig{
		Label:          "change::emergency",
		TrailerKey:     "Change-Type",
		TrailerValue:   "emergency",
		BranchPatterns: []string{"hotfix/*"},
	}
}

func TestCollectEmergencySignalsViaTrailer(t *testing.T) {
	repo := &Repository{
		changeLabels: map[string]struct{}{},
		commits: []repoCommit{
			{hash: "aaa1", message: "fix: urgent patch\n\nChange-Type: emergency"},
		},
	}
	repo.CollectEmergencySignals(nil, emergencyCfg())
	_, flagged := repo.changeLabels["change::emergency"]
	assert.True(t, flagged)
}

func TestCollectEmergencySignalsTrailerNotInLastParagraph(t *testing.T) {
	repo := &Repository{
		changeLabels: map[string]struct{}{},
		commits: []repoCommit{
			{hash: "aaa1", message: "fix: urgent patch\n\nChange-Type: emergency\n\nmore context"},
		},
	}
	repo.CollectEmergencySignals(nil, emergencyCfg())
	_, flagged := repo.changeLabels["change::emergency"]
	assert.False(t, flagged)
}

func TestCollectEmergencySignalsViaBranch(t *testing.T) {
	backend := &emergencyBackend{mrsBySha: map[string]*MergeRequestRef{
		"aaa1": {IID: 5, SourceBranch: "hotfix/payment", Labels: nil},
	}}
	repo := &Repository{
		changeLabels: map[string]struct{}{},
		commits:      []repoCommit{{hash: "aaa1", message: "fix: patch"}},
	}
	repo.CollectEmergencySignals(backend, emergencyCfg())
	_, flagged := repo.changeLabels["change::emergency"]
	assert.True(t, flagged)
}

func TestCollectEmergencySignalsViaExistingMRLabel(t *testing.T) {
	backend := &emergencyBackend{mrsBySha: map[string]*MergeRequestRef{
		"aaa1": {IID: 5, SourceBranch: "fix/payment", Labels: []string{"change::emergency"}},
	}}
	repo := &Repository{
		changeLabels: map[string]struct{}{},
		commits:      []repoCommit{{hash: "aaa1", message: "fix: patch"}},
	}
	repo.CollectEmergencySignals(backend, emergencyCfg())
	_, flagged := repo.changeLabels["change::emergency"]
	assert.True(t, flagged)
}

func TestCollectEmergencySignalsViaSquashedCommit(t *testing.T) {
	backend := &emergencyBackend{
		mrsBySha: map[string]*MergeRequestRef{
			"squash1": {IID: 7, SourceBranch: "fix/payment", Labels: nil},
		},
		commitsByIID: map[int][]string{
			7: {"fix: part one", "fix: part two\n\nChange-Type: emergency"},
		},
	}
	repo := &Repository{
		changeLabels: map[string]struct{}{},
		commits:      []repoCommit{{hash: "squash1", message: "fix: patch (squash commit)"}},
	}
	repo.CollectEmergencySignals(backend, emergencyCfg())
	_, flagged := repo.changeLabels["change::emergency"]
	assert.True(t, flagged)
	assert.Equal(t, 1, backend.commitCalls[7])
}

func TestCollectEmergencySignalsNoMatch(t *testing.T) {
	backend := &emergencyBackend{mrsBySha: map[string]*MergeRequestRef{
		"aaa1": {IID: 5, SourceBranch: "feature/payment", Labels: []string{"keep-me"}},
	}}
	repo := &Repository{
		changeLabels: map[string]struct{}{},
		commits:      []repoCommit{{hash: "aaa1", message: "fix: patch"}},
	}
	repo.CollectEmergencySignals(backend, emergencyCfg())
	_, flagged := repo.changeLabels["change::emergency"]
	assert.False(t, flagged)
}

func TestCollectEmergencySignalsAggregatesAcrossCommits(t *testing.T) {
	backend := &emergencyBackend{mrsBySha: map[string]*MergeRequestRef{
		"ordinary": {IID: 1, SourceBranch: "feature/payment"},
		"urgent":   {IID: 2, SourceBranch: "hotfix/payment"},
	}}
	repo := &Repository{
		changeLabels: map[string]struct{}{},
		commits: []repoCommit{
			{hash: "ordinary", message: "fix: regular fix"},
			{hash: "urgent", message: "fix: urgent fix"},
		},
	}
	repo.CollectEmergencySignals(backend, emergencyCfg())
	_, flagged := repo.changeLabels["change::emergency"]
	assert.True(t, flagged)
}

func TestCollectEmergencySignalsCachesPerMergeRequest(t *testing.T) {
	backend := &emergencyBackend{
		mrsBySha: map[string]*MergeRequestRef{
			"sha1": {IID: 9, SourceBranch: "fix/payment"},
			"sha2": {IID: 9, SourceBranch: "fix/payment"},
		},
		commitsByIID: map[int][]string{
			9: {"fix: part one", "fix: part two"},
		},
	}
	repo := &Repository{
		changeLabels: map[string]struct{}{},
		commits: []repoCommit{
			{hash: "sha1", message: "fix: part one"},
			{hash: "sha2", message: "fix: part two"},
		},
	}
	repo.CollectEmergencySignals(backend, emergencyCfg())
	assert.Equal(t, 1, backend.commitCalls[9])
}

func TestCollectEmergencySignalsSkipsAPIOnceFlagged(t *testing.T) {
	backend := &emergencyBackend{
		mrsBySha: map[string]*MergeRequestRef{
			"sha1": {IID: 11, SourceBranch: "hotfix/payment"},
		},
	}
	repo := &Repository{
		changeLabels: map[string]struct{}{},
		commits: []repoCommit{
			{hash: "sha1", message: "fix: part one"},
		},
	}
	repo.CollectEmergencySignals(backend, emergencyCfg())
	assert.Equal(t, 0, backend.commitCalls[11])
}

func TestCollectEmergencySignalsDisabledWithoutLabel(t *testing.T) {
	repo := &Repository{
		changeLabels: map[string]struct{}{},
		commits:      []repoCommit{{hash: "aaa1", message: "fix: urgent\n\nChange-Type: emergency"}},
	}
	cfg := emergencyCfg()
	cfg.Label = ""
	repo.CollectEmergencySignals(nil, cfg)
	assert.Empty(t, repo.changeLabels)
}
