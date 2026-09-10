package internal

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGitlab(t *testing.T) {
	testmux := http.NewServeMux()
	testserver := httptest.NewServer(testmux)
	defer testserver.Close()

	gitlab := NewGitlabBackend("test-token", "server", "my/test/repo")

	gitlab.server = testserver.URL
	assert.Error(t, gitlab.request(http.MethodGet, "notfound", http.StatusAccepted, nil, nil))
	assert.NoError(t, gitlab.request(http.MethodGet, "notfound", http.StatusNotFound, nil, nil))

	testmux.HandleFunc("/api/v4/testbody", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"foo": "bar"}`)
	})
	var body struct {
		Foo string `json:"foo"`
	}
	assert.NoError(t, gitlab.request(http.MethodGet, "testbody", http.StatusOK, nil, &body))
	assert.Equal(t, "bar", body.Foo)

	testmux.HandleFunc("/api/v4/brokenbody", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `-invalidjson-`)
	})
	assert.Error(t, gitlab.request(http.MethodGet, "brokenbody", http.StatusOK, nil, &body))

	noMrs := true
	testmux.HandleFunc("/api/v4/projects/my%2ftest%2frepo/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			return
		}
		if noMrs {
			_, _ = fmt.Fprint(w, `[]`)
			return
		}
		_, _ = fmt.Fprint(w, `[{
			"id": 123,
			"iid": 3,
			"source_branch": "semanticore/release",
			"state": "opened",
			"labels": ["keep-me", "change::standard"]
		}]`)
	})

	num, labels, err := gitlab.findOpenMergeRequest()
	assert.ErrorIs(t, err, errNoMergeRequestFound)
	assert.Equal(t, 0, num)
	assert.Nil(t, labels)

	noMrs = false
	num, labels, err = gitlab.findOpenMergeRequest()
	assert.NoError(t, err)
	assert.Equal(t, 3, num)
	assert.Equal(t, []string{"keep-me", "change::standard"}, labels)

	testmux.HandleFunc("/api/v4/projects/my%2ftest%2frepo/merge_requests/3", func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, r.ParseForm())
		if r.Method == http.MethodPut && r.Form.Get("state_event") == "" {
			assert.Equal(t, "keep-me,change::major", r.Form.Get("labels"))
		}
	})
	assert.NoError(t, gitlab.CloseMergeRequest())

	assert.NoError(t, gitlab.MergeRequest("main", "Release v1.2.3", "release description", "tag1,tag2,change::major"))
	noMrs = true
	assert.NoError(t, gitlab.MergeRequest("main", "Release v1.2.3", "release description", "tag1,tag2,change::major"))

	testmux.HandleFunc("/api/v4/projects/my%2ftest%2frepo/repository/tags", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	testmux.HandleFunc("/api/v4/projects/my%2ftest%2frepo/releases", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	assert.NoError(t, gitlab.CreateTag("v1.2.3", "abc123"))
	assert.NoError(t, gitlab.CreateRelease("v1.2.3", "changelog"))

	testmux.HandleFunc("/api/v4/projects/my%2ftest%2frepo", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"default_branch": "main"}`)
	})
	branch, err := gitlab.MainBranch()
	assert.NoError(t, err)
	assert.Equal(t, "main", branch)
}

func TestGitlabCommitMergeRequestAndRelatedCommits(t *testing.T) {
	testmux := http.NewServeMux()
	testserver := httptest.NewServer(testmux)
	defer testserver.Close()

	gitlab := NewGitlabBackend("test-token", "server", "my/test/repo")
	gitlab.server = testserver.URL

	testmux.HandleFunc("/api/v4/projects/my%2ftest%2frepo/repository/commits/sha-no-mr/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `[]`)
	})
	mr, err := gitlab.CommitMergeRequest("sha-no-mr")
	assert.NoError(t, err)
	assert.Nil(t, mr)

	testmux.HandleFunc("/api/v4/projects/my%2ftest%2frepo/repository/commits/sha-with-mr/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `[{
			"iid": 7,
			"source_branch": "hotfix/payment",
			"state": "merged",
			"labels": ["change::emergency", "keep-me"]
		}]`)
	})
	mr, err = gitlab.CommitMergeRequest("sha-with-mr")
	assert.NoError(t, err)
	assert.Equal(t, &MergeRequestRef{IID: 7, SourceBranch: "hotfix/payment", Labels: []string{"change::emergency", "keep-me"}}, mr)

	testmux.HandleFunc("/api/v4/projects/my%2ftest%2frepo/merge_requests/7/commits", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `[{"message": "fix: part one"}, {"message": "fix: part two\n\nChange-Type: emergency"}]`)
	})
	messages, err := gitlab.MergeRequestCommitMessages(7)
	assert.NoError(t, err)
	assert.Equal(t, []string{"fix: part one", "fix: part two\n\nChange-Type: emergency"}, messages)
}

func TestGitlabCurrentChangeLabels(t *testing.T) {
	testmux := http.NewServeMux()
	testserver := httptest.NewServer(testmux)
	defer testserver.Close()

	gitlab := NewGitlabBackend("test-token", "server", "my/test/repo")
	gitlab.server = testserver.URL

	testmux.HandleFunc("/api/v4/projects/my%2ftest%2frepo/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `[{
			"id": 123,
			"iid": 3,
			"source_branch": "semanticore/release",
			"state": "opened",
			"labels": ["keep-me", "change::emergency"]
		}]`)
	})
	labels, err := gitlab.CurrentChangeLabels("change::")
	assert.NoError(t, err)
	assert.Equal(t, []string{"change::emergency"}, labels)
}
