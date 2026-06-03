package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/fengxudong/harness/internal/model"
	"github.com/fengxudong/harness/internal/state"
	"github.com/fengxudong/harness/internal/store"
)

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init", "-q")
	run(t, dir, "git", "config", "user.email", "t@example.com")
	run(t, dir, "git", "config", "user.name", "tester")
	return dir
}

func canonRoot(t *testing.T, dir string) string {
	t.Helper()
	root, err := repoRoot(dir)
	if err != nil {
		t.Fatalf("repoRoot: %v", err)
	}
	return root
}

func TestInit_RejectsNonGitRepo(t *testing.T) {
	dir := t.TempDir() // not a git repo
	err := Init(dir)
	var ce *CodedError
	if !errors.As(err, &ce) || ce.Code != ExitUsage {
		t.Fatalf("want ExitUsage CodedError, got %v", err)
	}
}

func TestInit_RejectsTrackedHarness(t *testing.T) {
	dir := gitRepo(t)
	// Commit a file under .harness so it becomes tracked.
	if err := os.MkdirAll(filepath.Join(dir, ".harness"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".harness", "x"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", ".harness/x")
	run(t, dir, "git", "commit", "-qm", "track harness")

	err := Init(dir)
	var ce *CodedError
	if !errors.As(err, &ce) || ce.Code != ExitStateCorrupt {
		t.Fatalf("want ExitStateCorrupt, got %v", err)
	}
}

func TestInit_WritesArtifacts(t *testing.T) {
	dir := gitRepo(t)
	if err := Init(dir); err != nil {
		t.Fatalf("init: %v", err)
	}
	root := canonRoot(t, dir)
	l := store.NewLayout(root)

	for _, f := range []string{"job.schema.json", "event.schema.json", "task.schema.json"} {
		if _, err := os.Stat(filepath.Join(l.Schemas(), f)); err != nil {
			t.Errorf("missing schema %s: %v", f, err)
		}
	}
	if _, err := os.Stat(l.Reserved()); err != nil {
		t.Errorf("missing reserved.json: %v", err)
	}
	if _, err := os.Stat(l.Contract()); err != nil {
		t.Errorf("missing contract: %v", err)
	}
	gi, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	if !contains(string(gi), ".harness/") || !contains(string(gi), ".worktrees/") {
		t.Errorf(".gitignore missing entries:\n%s", gi)
	}
}

func TestSessionStart_CapturesBaseline(t *testing.T) {
	dir := gitRepo(t)
	// one commit so HEAD exists, plus an untracked file so baseline is dirty
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", "README")
	run(t, dir, "git", "commit", "-qm", "init")
	if err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}

	sid, err := SessionStart(dir)
	if err != nil {
		t.Fatalf("session start: %v", err)
	}
	l := store.NewLayout(canonRoot(t, dir))

	var bl baseline
	if err := store.ReadJSON(l.SessionBaseline(sid), &bl); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if bl.HEAD == "" {
		t.Error("baseline HEAD empty (expected a commit)")
	}
	if len(bl.Porcelain) == 0 {
		t.Error("baseline porcelain empty (expected dirty.txt)")
	}
	if _, err := os.Stat(l.ActiveTask(sid)); err != nil {
		t.Errorf("active-task.json missing: %v", err)
	}
}

func TestRecover_RebuildsViews(t *testing.T) {
	dir := gitRepo(t)
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	sid, err := SessionStart(dir)
	if err != nil {
		t.Fatal(err)
	}
	l := store.NewLayout(canonRoot(t, dir))

	// produce a job + transition via the engine (writes events + views)
	eng := state.New(l, sid)
	j := model.Job{
		JobID: "J-1", TaskID: "T-1", CreatedBy: "codex", TargetRuntime: model.RuntimeClaude,
		Role: model.RoleImplementation, Writes: true, Mode: model.ModeWorktree,
		StateRoot: l.StateRoot, RepoRoot: l.RepoRoot, Workdir: "/x",
		Scope: model.Scope{Allowed: []string{"src/**"}, Denied: []string{}},
		Delegation: model.Delegation{Depth: 1, ChainFingerprints: []string{"T-1:x"}},
		Budget: model.JobBudget{MaxTokens: 1, TimeoutS: 1}, ResultContract: "job-result.schema.json",
	}
	if err := eng.CreateJob("codex", j); err != nil {
		t.Fatal(err)
	}
	// drive to a terminal state so recover's stale reconciliation leaves it alone
	if _, err := eng.TransitionJob("claude", "J-1", 1, model.JobRunning); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.TransitionJob("claude", "J-1", 2, model.JobCompleted); err != nil {
		t.Fatal(err)
	}

	// wipe views, recover should rebuild from events
	if err := os.RemoveAll(l.Views(sid)); err != nil {
		t.Fatal(err)
	}
	if err := Recover(dir, sid); err != nil {
		t.Fatalf("recover: %v", err)
	}
	var view model.Job
	if err := store.ReadJSON(l.JobView(sid, "J-1"), &view); err != nil {
		t.Fatalf("view not rebuilt: %v", err)
	}
	if view.Status != model.JobCompleted || view.Rev != 3 {
		t.Fatalf("rebuilt view wrong: %+v", view)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}
