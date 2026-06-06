// Package worktree manages per-write-job git worktrees: each write job runs in
// its own worktree on branch job/<jid> so writes never touch the human's main
// tree and crash recovery is just discarding the worktree (rev3 §7, §15).
package worktree

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Info describes a created worktree.
type Info struct {
	Workdir    string
	Branch     string
	BaseCommit string
}

// Path is the on-disk location of a job's worktree.
func Path(repoRoot, jid string) string {
	return filepath.Join(repoRoot, ".worktrees", jid)
}

// Branch is the branch name for a job's worktree.
func Branch(jid string) string { return "job/" + jid }

// Add creates a worktree on a fresh branch off HEAD and returns its binding.
// base_commit is a real commit (not a stash), so it is durable and gc-safe
// (rev3 §7, fixes N8).
func Add(repoRoot, jid string) (Info, error) {
	head, err := git(repoRoot, "rev-parse", "HEAD")
	if err != nil {
		return Info{}, fmt.Errorf("worktree add needs a base commit (no HEAD): %s", head)
	}
	branch := Branch(jid)
	workdir := Path(repoRoot, jid)
	if out, err := git(repoRoot, "worktree", "add", "-b", branch, workdir, head); err != nil {
		return Info{}, fmt.Errorf("git worktree add: %s", out)
	}
	return Info{Workdir: workdir, Branch: branch, BaseCommit: head}, nil
}

// Remove tears a worktree down idempotently: remove --force, prune, then delete
// the residual branch so a later re-dispatch can re-add the same job id without
// a "branch already exists" fatal (rev3 §15, fixes N16). Use for abandoned /
// stale / rejected jobs.
func Remove(repoRoot, jid string) error {
	_, _ = git(repoRoot, "worktree", "remove", "--force", Path(repoRoot, jid))
	_, _ = git(repoRoot, "worktree", "prune")
	_, _ = git(repoRoot, "branch", "-D", Branch(jid))
	return nil
}

// RemoveWorktreeOnly reclaims the disk-heavy file checkout but KEEPS the
// job/<jid> branch, so the work stays referenceable and re-integratable. Used to
// stop worktrees accumulating after a successful integrate and by `harness prune`.
func RemoveWorktreeOnly(repoRoot, jid string) error {
	_, _ = git(repoRoot, "worktree", "remove", "--force", Path(repoRoot, jid))
	_, _ = git(repoRoot, "worktree", "prune")
	return nil
}

func git(repoRoot string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
