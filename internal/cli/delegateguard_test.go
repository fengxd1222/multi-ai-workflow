package cli

import (
	"testing"

	"github.com/fengxd1222/multi-ai-workflow/internal/model"
	"github.com/fengxd1222/multi-ai-workflow/internal/state"
	"github.com/fengxd1222/multi-ai-workflow/internal/store"
)

func TestDelegate_CycleDetected(t *testing.T) {
	dir, sid, _ := initSessionWithCommit(t)
	// the target fingerprint already appears in the ancestor chain
	_, err := Delegate(dir, sid, DelegateSpec{
		TaskID: "T-1", Role: model.RoleImplementation, Runtime: model.RuntimeClaude,
		ParentChain: []string{"T-1:implementation"},
	})
	if CodeOf(err) != ExitDelegationLoop {
		t.Fatalf("want ExitDelegationLoop for cycle, got %v", err)
	}
}

func TestDelegate_BudgetExceeded(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	l := store.NewLayout(root)
	eng := state.New(l, sid)
	if err := eng.CreateTask("o", model.Task{
		TaskID: "T-1", Title: "x", Acceptance: []string{"a"}, JobIDs: []string{},
		Budget: &model.Budget{MaxTokens: 100},
	}); err != nil {
		t.Fatal(err)
	}
	if err := eng.CreateJob("c", model.Job{
		JobID: "J-1", TaskID: "T-1", CreatedBy: "c", TargetRuntime: model.RuntimeClaude,
		Role: model.RoleImplementation, Writes: true, Mode: model.ModeWorktree,
		StateRoot: l.StateRoot, RepoRoot: root, Workdir: root,
		Scope:      model.Scope{Allowed: []string{"src/**"}},
		Delegation: model.Delegation{Depth: 1, ChainFingerprints: []string{"T-1:x"}},
		Budget:     model.JobBudget{TimeoutS: 1}, ResultContract: "job-result.schema.json",
	}); err != nil {
		t.Fatal(err)
	}
	// usage events for the task's job push it over budget
	_ = eng.AppendInfo("worker:J-1", model.EvUsageReported, map[string]any{"job_id": "J-1", "tokens": 150})

	if _, err := Delegate(dir, sid, DelegateSpec{
		TaskID: "T-1", Role: model.RoleTest, Runtime: model.RuntimeClaude,
	}); CodeOf(err) != ExitBudgetExceeded {
		t.Fatalf("want ExitBudgetExceeded, got %v", err)
	}
}
