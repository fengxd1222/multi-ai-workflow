package cli

import (
	"os"
	"testing"

	"github.com/fengxudong/harness/internal/model"
	"github.com/fengxudong/harness/internal/state"
	"github.com/fengxudong/harness/internal/store"
	"github.com/fengxudong/harness/internal/worktree"
)

// TestRecover_StaleJobDiscardsWorktree: a running job whose recorded boot_id no
// longer matches is stale; recover discards its worktree and resets it (N5).
func TestRecover_StaleJobDiscardsWorktree(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	l, workdir := runningWriteJob(t, sid, root) // Worker boot "b" != real boot -> stale

	if _, err := os.Stat(workdir); err != nil {
		t.Fatalf("worktree should exist before recover: %v", err)
	}
	if err := Recover(dir, sid); err != nil {
		t.Fatal(err)
	}
	var j model.Job
	_ = store.ReadJSON(l.JobView(sid, "J-1"), &j)
	if j.Status != model.JobCreated || j.RecoverCount != 1 {
		t.Fatalf("stale job not reset: status=%s count=%d", j.Status, j.RecoverCount)
	}
	if _, err := os.Stat(workdir); !os.IsNotExist(err) {
		t.Fatal("stale worktree should be discarded")
	}
}

// TestRecover_AliveJobUntouched: a running job whose worker is this live process
// must not be recovered (N5).
func TestRecover_AliveJobUntouched(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	l := store.NewLayout(root)
	eng := state.New(l, sid)
	if err := eng.CreateJob("c", model.Job{
		JobID: "J-2", TaskID: "T-1", CreatedBy: "c", TargetRuntime: model.RuntimeClaude,
		Role: model.RoleImplementation, Writes: true, Mode: model.ModeWorktree,
		StateRoot: l.StateRoot, RepoRoot: root, Workdir: root,
		Scope:      model.Scope{Allowed: []string{"src/**"}},
		Delegation: model.Delegation{Depth: 1, ChainFingerprints: []string{"T-1:x"}},
		Budget:     model.JobBudget{TimeoutS: 2}, ResultContract: "job-result.schema.json",
	}); err != nil {
		t.Fatal(err)
	}
	wt, err := worktree.Add(root, "J-2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eng.TransitionJobRunning("a", "J-2", 1, &model.Worker{PID: os.Getpid(), BootID: bootID()}, wt.Workdir, wt.Branch, wt.BaseCommit); err != nil {
		t.Fatal(err)
	}

	if err := Recover(dir, sid); err != nil {
		t.Fatal(err)
	}
	var j model.Job
	_ = store.ReadJSON(l.JobView(sid, "J-2"), &j)
	if j.Status != model.JobRunning {
		t.Fatalf("alive job was wrongly recovered: status=%s", j.Status)
	}
}

// TestRecover_OrphanWorktreePruned: a git-registered worktree with no owning job
// is removed (N9).
func TestRecover_OrphanWorktreePruned(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	if _, err := worktree.Add(root, "J-orphan"); err != nil {
		t.Fatal(err)
	}
	orphanPath := worktree.Path(root, "J-orphan")
	if _, err := os.Stat(orphanPath); err != nil {
		t.Fatalf("orphan worktree should exist: %v", err)
	}
	if err := Recover(dir, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphanPath); !os.IsNotExist(err) {
		t.Fatal("orphan worktree should be pruned")
	}
}
