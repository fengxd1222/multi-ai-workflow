package cli

import (
	"os/exec"
	"strings"
)

// git is a thin wrapper around the git CLI run in dir.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// repoRoot returns the absolute toplevel of the git repo containing dir, or an
// error if dir is not inside a git repo (rev3 D6 hard prerequisite).
func repoRoot(dir string) (string, error) {
	return git(dir, "rev-parse", "--show-toplevel")
}

// isTracked reports whether a path (relative to repoRoot) is tracked by git.
// Used to reject a .harness that was accidentally committed (rev3 N35).
func isTracked(repoRoot, rel string) bool {
	_, err := git(repoRoot, "ls-files", "--error-unmatch", rel)
	return err == nil
}

// headCommit returns HEAD's sha, or "" if the repo has no commits yet.
func headCommit(repoRoot string) string {
	out, err := git(repoRoot, "rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return out
}

// porcelainBaseline captures the full dirty/untracked/ignored state so a session
// can tell which changes pre-date it (rev3 §2 session-baseline; reserved set).
func porcelainBaseline(repoRoot string) ([]string, error) {
	out, err := git(repoRoot, "status", "--porcelain", "--untracked-files=all", "--ignored")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return []string{}, nil
	}
	return strings.Split(out, "\n"), nil
}

// ignorecase reports whether the repo's filesystem is case-insensitive
// (core.ignorecase=true), which scope-eval uses for casefold matching (N32).
func ignorecase(repoRoot string) bool {
	out, _ := git(repoRoot, "config", "--get", "core.ignorecase")
	return out == "true"
}
