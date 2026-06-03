package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func repoWithCommit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	gitRun(t, dir, "config", "user.email", "t@t.t")
	gitRun(t, dir, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "f")
	gitRun(t, dir, "commit", "-qm", "base")
	return dir
}

func branchExists(t *testing.T, dir, branch string) bool {
	t.Helper()
	out, _ := exec.Command("git", "-C", dir, "branch", "--list", branch).Output()
	return strings.Contains(string(out), branch)
}

func TestAdd_CreatesWorktreeAndBranch(t *testing.T) {
	dir := repoWithCommit(t)
	info, err := Add(dir, "J-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(info.Workdir); err != nil {
		t.Fatalf("worktree dir missing: %v", err)
	}
	if !branchExists(t, dir, "job/J-1") {
		t.Fatal("branch job/J-1 not created")
	}
	if info.BaseCommit == "" {
		t.Fatal("base commit empty")
	}
}

// TestRemoveThenAdd_NoBranchResidueFatal is the N16 regression: after Remove,
// re-adding the same job id must not fail on a leftover branch.
func TestRemoveThenAdd_NoBranchResidueFatal(t *testing.T) {
	dir := repoWithCommit(t)
	if _, err := Add(dir, "J-1"); err != nil {
		t.Fatal(err)
	}
	if err := Remove(dir, "J-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(Path(dir, "J-1")); !os.IsNotExist(err) {
		t.Fatal("worktree dir should be gone after Remove")
	}
	if branchExists(t, dir, "job/J-1") {
		t.Fatal("branch should be deleted by Remove")
	}
	// Remove again is a no-op.
	if err := Remove(dir, "J-1"); err != nil {
		t.Fatalf("second Remove not idempotent: %v", err)
	}
	// Re-add must succeed (no branch-already-exists fatal).
	if _, err := Add(dir, "J-1"); err != nil {
		t.Fatalf("re-add after remove failed: %v", err)
	}
}

func TestAdd_NoHeadFails(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	if _, err := Add(dir, "J-1"); err == nil {
		t.Fatal("expected Add to fail on a repo with no commits")
	}
}
