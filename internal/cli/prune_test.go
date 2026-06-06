package cli

import (
	"os"
	"os/exec"
	"testing"

	"github.com/fengxd1222/multi-ai-workflow/internal/model"
	"github.com/fengxd1222/multi-ai-workflow/internal/state"
	"github.com/fengxd1222/multi-ai-workflow/internal/store"
	"github.com/fengxd1222/multi-ai-workflow/internal/worktree"
)

// TestPrune_ReclaimsTerminalKeepsRunning: prune removes the worktree of a
// completed write job (keeping its branch) but leaves a running job's worktree.
func TestPrune_ReclaimsTerminalKeepsRunning(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	l := store.NewLayout(root)
	eng := state.New(l, sid)

	mk := func(id string) {
		j := model.Job{
			JobID: id, TaskID: "T-1", CreatedBy: "c", TargetRuntime: model.RuntimeClaude,
			Role: model.RoleImplementation, Writes: true, Mode: model.ModeWorktree,
			StateRoot: l.StateRoot, RepoRoot: root, Workdir: root,
			Scope:      model.Scope{Allowed: []string{"src/**"}},
			Delegation: model.Delegation{Depth: 1, ChainFingerprints: []string{"T-1:x"}},
			Budget:     model.JobBudget{TimeoutS: 2}, ResultContract: "job-result.schema.json",
		}
		if err := eng.CreateJob("c", j); err != nil {
			t.Fatal(err)
		}
		wt, err := worktree.Add(root, id)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := eng.TransitionJobRunning("a", id, 1, &model.Worker{PID: 1, BootID: "b"}, wt.Workdir, wt.Branch, wt.BaseCommit); err != nil {
			t.Fatal(err)
		}
	}
	mk("J-done")
	mk("J-run")
	// J-done -> completed (rev 2 -> 3); J-run stays running
	if _, err := eng.TransitionJob("a", "J-done", 2, model.JobCompleted); err != nil {
		t.Fatal(err)
	}

	if err := Prune(dir, sid); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(worktree.Path(root, "J-done")); !os.IsNotExist(err) {
		t.Fatal("completed job's worktree should be pruned")
	}
	if _, err := os.Stat(worktree.Path(root, "J-run")); err != nil {
		t.Fatal("running job's worktree must be kept")
	}
	// branch of the pruned job is kept (re-integratable)
	out, _ := exec.Command("git", "-C", root, "branch", "--list", "job/J-done").Output()
	if len(out) == 0 {
		t.Fatal("pruned job's branch should be kept")
	}
}
