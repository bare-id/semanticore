package hook

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/go-git/go-git/v5"

	"github.com/bare-id/semanticore/internal"
)

// ReleaseScriptHook runs script via `sh -c` in the worktree root with
// SEMANTICORE_VERSION=X.Y.Z set and stages every path the script changed
// (new, modified, deleted or renamed files), so they land in the release commit.
//
// Only paths whose git status differs from before the script ran are staged,
// so pre-existing untracked files (e.g. CI artifacts) are not committed.
// A file that was already dirty before the hook and is changed again keeps the
// same status code and is therefore not restaged; CI checkouts are clean.
func ReleaseScriptHook(wt *git.Worktree, repository *internal.Repository, script string) error {
	if script == "" {
		return nil
	}

	before, err := wt.Status()
	if err != nil {
		return fmt.Errorf("release-hook: reading status: %w", err)
	}

	version := fmt.Sprintf("%d.%d.%d", repository.Major, repository.Minor, repository.Patch)
	log.Printf("[semanticore] running release hook for version %s", version)

	cmd := exec.Command("sh", "-c", script)
	cmd.Dir = wt.Filesystem.Root()
	cmd.Env = append(os.Environ(), "SEMANTICORE_VERSION="+version)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("release-hook: %w", err)
	}

	after, err := wt.Status()
	if err != nil {
		return fmt.Errorf("release-hook: reading status: %w", err)
	}

	for path, status := range after {
		if prev, ok := before[path]; ok && *prev == *status {
			continue
		}
		// wt.Add also stages the deletion of a path missing from the worktree
		if _, err := wt.Add(path); err != nil {
			return fmt.Errorf("release-hook: staging %s: %w", path, err)
		}
	}

	return nil
}
