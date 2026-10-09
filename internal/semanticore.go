package internal

import (
	"github.com/go-git/go-git/v5/plumbing/transport"
)

type Backend interface {
	transport.AuthMethod
	Release(tag, ref, changelog string) error
	MergeRequest(target, title, description, labels string) error
	CloseMergeRequest() error
	MainBranch() (string, error)
	// IssuePrefixedLabels returns all labels starting with prefix for the given issue number.
	// Implementations should return nil, nil when issues are not supported or the issue is not found.
	IssuePrefixedLabels(id int, prefix string) ([]string, error)
	// CommitMergeRequest returns the merge/pull request associated with the given commit SHA,
	// or nil, nil when no such request exists.
	CommitMergeRequest(sha string) (*MergeRequestRef, error)
	// MergeRequestCommitMessages returns the commit messages of a merge/pull request's own
	// commits, used to recover trailers lost by a squash merge.
	MergeRequestCommitMessages(iid int) ([]string, error)
	// CurrentChangeLabels returns the labels starting with prefix already present on the
	// bot-managed release merge/pull request, or nil, nil when it does not exist yet.
	CurrentChangeLabels(prefix string) ([]string, error)
}

// MergeRequestRef describes the merge/pull request associated with a commit.
type MergeRequestRef struct {
	IID          int
	SourceBranch string
	Labels       []string
}
