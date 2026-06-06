package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fengxudong/harness/internal/model"
	"github.com/fengxudong/harness/internal/state"
	"github.com/fengxudong/harness/internal/store"
)

// TestRun_TrellisJournalWriteBack: a job carrying a trellis_task triggers a
// best-effort add_session.py call after run. We stub the script (self-executable
// sh) to drop a marker file and assert it was invoked with the journal args.
func TestRun_TrellisJournalWriteBack(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)

	// stub Trellis: a task + a fake add_session.py that records its invocation
	td := filepath.Join(root, ".trellis", "tasks", "02-27-x")
	if err := os.MkdirAll(td, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(td, "task.json"), []byte(`{"id":"02-27-x","title":"x","status":"planning"}`), 0o644)
	sd := filepath.Join(root, ".trellis", "scripts")
	os.MkdirAll(sd, 0o755)
	marker := filepath.Join(root, "session-recorded.txt")
	os.WriteFile(filepath.Join(sd, "add_session.py"), []byte("#!/bin/sh\necho \"$*\" > "+marker+"\n"), 0o755)

	eng := state.New(store.NewLayout(root), sid)
	job := model.Job{
		JobID: "J-1", TaskID: "T-1", CreatedBy: "x", TargetRuntime: model.RuntimeClaude,
		Role: model.RoleImplementation, Writes: true, Mode: model.ModeWorktree, TrellisTask: "02-27-x",
		StateRoot: root + "/.harness", RepoRoot: root, Workdir: root,
		Scope:      model.Scope{Allowed: []string{"src/**"}},
		Delegation: model.Delegation{Depth: 1, ChainFingerprints: []string{"T-1:x"}},
		Budget:     model.JobBudget{TimeoutS: 2}, ResultContract: "job-result.schema.json",
	}
	if err := eng.CreateJob("x", job); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_CLAUDE_BIN", "harness-no-such-bin") // job will fail; write-back still fires
	_ = Run(dir, sid, "J-1")

	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("add_session.py was not invoked: %v", err)
	}
	if s := string(data); !contains(s, "--title") || !contains(s, "--summary") {
		t.Fatalf("journal call missing args: %q", s)
	}
}

func initSessionWithCommit(t *testing.T) (dir, sid, root string) {
	t.Helper()
	dir = gitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "seed"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", "seed")
	run(t, dir, "git", "commit", "-qm", "base")
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	s, err := SessionStart(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, s, canonRoot(t, dir)
}

func TestRun_CodexMissingBinDrivesJobToFailed(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	eng := state.New(store.NewLayout(root), sid)
	job := model.Job{
		JobID: "J-1", TaskID: "T-1", CreatedBy: "x", TargetRuntime: model.RuntimeCodex,
		Role: model.RoleImplementation, Writes: true, Mode: model.ModeWorktree,
		StateRoot: root + "/.harness", RepoRoot: root, Workdir: root,
		Scope:      model.Scope{Allowed: []string{"src/**"}},
		Delegation: model.Delegation{Depth: 1, ChainFingerprints: []string{"T-1:x"}},
		Budget:     model.JobBudget{TimeoutS: 2}, ResultContract: "job-result.schema.json",
	}
	if err := eng.CreateJob("x", job); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_CODEX_BIN", "harness-no-such-codex")
	if err := Run(dir, sid, "J-1"); err != nil {
		t.Fatalf("Run should complete: %v", err)
	}
	var v model.Job
	_ = store.ReadJSON(store.NewLayout(root).JobView(sid, "J-1"), &v)
	if v.Status != model.JobFailed {
		t.Fatalf("status=%s want failed", v.Status)
	}
}

func TestRun_JobNotFound(t *testing.T) {
	dir, sid, _ := initSessionWithCommit(t)
	if err := Run(dir, sid, "J-missing"); CodeOf(err) != ExitUsage {
		t.Fatalf("want ExitUsage for missing job, got %v", err)
	}
}

func TestRun_UnknownRuntime(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	eng := state.New(store.NewLayout(root), sid)
	job := model.Job{
		JobID: "J-1", TaskID: "T-1", CreatedBy: "x", TargetRuntime: "bogus",
		Role: model.RoleAnalysis, Writes: false, Mode: model.ModeShared,
		StateRoot: root + "/.harness", RepoRoot: root, Workdir: root,
		Scope:      model.Scope{Allowed: []string{"src/**"}},
		Delegation: model.Delegation{Depth: 1, ChainFingerprints: []string{"T-1:x"}},
		Budget:     model.JobBudget{TimeoutS: 1}, ResultContract: "job-result.schema.json",
	}
	if err := eng.CreateJob("x", job); err != nil {
		t.Fatal(err)
	}
	if err := Run(dir, sid, "J-1"); CodeOf(err) != ExitUsage {
		t.Fatalf("want ExitUsage for unknown runtime, got %v", err)
	}
}

// TestRun_MissingBinDrivesJobToFailed exercises the full CLI -> adapter ->
// worktree -> runtime wiring deterministically: the claude binary is pointed at
// a non-existent path, so the worker process fails to start and the job ends
// failed (runtime-exec-failed) — no real CLI required.
func TestRun_MissingBinDrivesJobToFailed(t *testing.T) {
	dir := gitRepo(t)
	// need a commit so the worktree can branch off HEAD
	if err := os.WriteFile(filepath.Join(dir, "seed"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", "seed")
	run(t, dir, "git", "commit", "-qm", "base")

	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	sid, err := SessionStart(dir)
	if err != nil {
		t.Fatal(err)
	}
	root := canonRoot(t, dir)
	l := store.NewLayout(root)
	eng := state.New(l, sid)

	job := model.Job{
		JobID: "J-1", TaskID: "T-1", CreatedBy: "codex", TargetRuntime: model.RuntimeClaude,
		Role: model.RoleImplementation, Writes: true, Mode: model.ModeWorktree,
		StateRoot: l.StateRoot, RepoRoot: root, Workdir: root,
		Scope:      model.Scope{Allowed: []string{"src/**"}},
		Delegation: model.Delegation{Depth: 1, ChainFingerprints: []string{"T-1:x"}},
		Budget:     model.JobBudget{MaxTokens: 1, TimeoutS: 2}, ResultContract: "job-result.schema.json",
	}
	if err := eng.CreateJob("codex", job); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HARNESS_CLAUDE_BIN", "harness-no-such-bin-xyz")
	if err := Run(dir, sid, "J-1"); err != nil {
		t.Fatalf("Run should complete (job failed, not error): %v", err)
	}

	var view model.Job
	if err := store.ReadJSON(l.JobView(sid, "J-1"), &view); err != nil {
		t.Fatal(err)
	}
	if view.Status != model.JobFailed {
		t.Fatalf("job status=%s want failed", view.Status)
	}
}
