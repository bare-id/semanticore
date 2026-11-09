package hook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bare-id/semanticore/internal"
)

func setupScriptRepo(t *testing.T) (string, *git.Worktree, *internal.Repository) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)

	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name+"\n"), 0644))
		_, err = wt.Add(name)
		require.NoError(t, err)
	}
	sig := &object.Signature{Name: "testing", Email: "testing@example.com"}
	_, err = wt.Commit("test(semanticore): initial commit", &git.CommitOptions{Author: sig, Committer: sig})
	require.NoError(t, err)

	// untracked file present before the hook runs, must not be staged
	require.NoError(t, os.WriteFile(filepath.Join(dir, "artifact.txt"), []byte("build output\n"), 0644))

	repository, err := internal.ReadRepository(repo, true)
	require.NoError(t, err)
	repository.Major = 4
	repository.Minor = 5
	repository.Patch = 6
	return dir, wt, repository
}

func TestReleaseScriptHook(t *testing.T) {
	dir, wt, repository := setupScriptRepo(t)

	script := `echo changed >> a.txt && rm b.txt && mv c.txt d.txt && printf %s "$SEMANTICORE_VERSION" > VERSION`
	require.NoError(t, ReleaseScriptHook(wt, repository, script))

	version, err := os.ReadFile(filepath.Join(dir, "VERSION"))
	require.NoError(t, err)
	assert.Equal(t, "4.5.6", string(version))

	status, err := wt.Status()
	require.NoError(t, err)
	assert.Equal(t, git.Modified, status.File("a.txt").Staging)
	assert.Equal(t, git.Deleted, status.File("b.txt").Staging)
	assert.Equal(t, git.Deleted, status.File("c.txt").Staging)
	assert.Equal(t, git.Added, status.File("d.txt").Staging)
	assert.Equal(t, git.Added, status.File("VERSION").Staging)
	assert.Equal(t, git.Untracked, status.File("artifact.txt").Staging)
	assert.Equal(t, git.Untracked, status.File("artifact.txt").Worktree)
}

func TestReleaseScriptHookIgnoresGitignored(t *testing.T) {
	dir, wt, repository := setupScriptRepo(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("dist/\n"), 0644))
	_, err := wt.Add(".gitignore")
	require.NoError(t, err)

	script := `mkdir -p dist && echo output > dist/out.txt && echo changed >> a.txt`
	require.NoError(t, ReleaseScriptHook(wt, repository, script))

	status, err := wt.Status()
	require.NoError(t, err)
	assert.Equal(t, git.Modified, status.File("a.txt").Staging)

	// ignored output must not be in the index
	_, err = os.Stat(filepath.Join(dir, "dist", "out.txt"))
	require.NoError(t, err, "script should have created dist/out.txt")
	repo, err := git.PlainOpen(dir)
	require.NoError(t, err)
	index, err := repo.Storer.Index()
	require.NoError(t, err)
	_, err = index.Entry("dist/out.txt")
	assert.Error(t, err, "ignored file dist/out.txt was staged")
}

func TestReleaseScriptHookFailure(t *testing.T) {
	_, wt, repository := setupScriptRepo(t)
	assert.Error(t, ReleaseScriptHook(wt, repository, "exit 1"))
}

func TestReleaseScriptHookEmpty(t *testing.T) {
	_, wt, repository := setupScriptRepo(t)

	before, err := wt.Status()
	require.NoError(t, err)
	require.NoError(t, ReleaseScriptHook(wt, repository, ""))
	after, err := wt.Status()
	require.NoError(t, err)
	assert.Equal(t, before.String(), after.String())
}
