package cli

import (
	"testing"

	"github.com/fengxd1222/multi-ai-workflow/internal/model"
	"github.com/fengxd1222/multi-ai-workflow/internal/state"
	"github.com/fengxd1222/multi-ai-workflow/internal/store"
)

// TestGateReject_CancelsJobUnblocksContract is the finding-1/2 closed loop: a
// needs-human job blocks completion; rejecting its gate cancels the job, and a
// separate completed job then satisfies the contract.
func TestGateReject_CancelsJobUnblocksContract(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	l := store.NewLayout(root)
	eng := state.New(l, sid)

	if err := eng.CreateTask("o", model.Task{TaskID: "T-1", Title: "x", Acceptance: []string{"a"}, JobIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	mk := func(id string) model.Job {
		return model.Job{
			JobID: id, TaskID: "T-1", CreatedBy: "c", TargetRuntime: model.RuntimeClaude,
			Role: model.RoleImplementation, Writes: true, Mode: model.ModeWorktree,
			StateRoot: l.StateRoot, RepoRoot: root, Workdir: root,
			Scope:      model.Scope{Allowed: []string{"src/**"}},
			Delegation: model.Delegation{Depth: 1, ChainFingerprints: []string{"T-1:x"}},
			Budget:     model.JobBudget{TimeoutS: 1}, ResultContract: "job-result.schema.json",
		}
	}
	// J-1 ends needs-human with a gate
	if err := eng.CreateJob("c", mk("J-1")); err != nil {
		t.Fatal(err)
	}
	_, _ = eng.TransitionJob("a", "J-1", 1, model.JobRunning)
	_, _ = eng.TransitionJob("a", "J-1", 2, model.JobNeedsHuman)
	g, err := eng.OpenGateForJob("a", "T-1", "J-1", "scope-violation", nil)
	if err != nil {
		t.Fatal(err)
	}
	// J-2 completes
	if err := eng.CreateJob("c", mk("J-2")); err != nil {
		t.Fatal(err)
	}
	_, _ = eng.TransitionJob("a", "J-2", 1, model.JobRunning)
	_, _ = eng.TransitionJob("a", "J-2", 2, model.JobCompleted)

	// before reject: J-1 needs-human blocks AllJobsDone
	if c, _ := ComputeContract(l, sid, "T-1"); c.AllJobsDone {
		t.Fatal("needs-human job should block AllJobsDone")
	}

	if err := GateReject(dir, sid, g.GateID); err != nil {
		t.Fatal(err)
	}

	var j model.Job
	_ = store.ReadJSON(l.JobView(sid, "J-1"), &j)
	if j.Status != model.JobCancelled {
		t.Fatalf("rejected job status = %s want cancelled", j.Status)
	}
	// after cancel: only J-2 (completed) counts -> AllJobsDone
	if c, _ := ComputeContract(l, sid, "T-1"); !c.AllJobsDone {
		t.Fatal("after cancelling J-1, J-2 completed should satisfy AllJobsDone")
	}
}

func TestGateApprove_ResetsNeedsHumanJob(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	l, _ := runningWriteJob(t, sid, root) // J-1 running rev2, scope src/** deny package.json
	eng := state.New(l, sid)
	_, _ = eng.TransitionJob("a", "J-1", 2, model.JobNeedsHuman)
	g, err := eng.OpenGateForJob("a", "T-1", "J-1", "scope-violation", []string{"package.json"})
	if err != nil {
		t.Fatal(err)
	}

	if err := GateApprove(dir, sid, g.GateID, "approve_extra_files", []string{"package.json"}); err != nil {
		t.Fatal(err)
	}
	var j model.Job
	_ = store.ReadJSON(l.JobView(sid, "J-1"), &j)
	if j.Status != model.JobCreated {
		t.Fatalf("approved needs-human job should reset to created, got %s", j.Status)
	}
	if !sliceHas(j.Scope.Allowed, "package.json") {
		t.Fatalf("approve_extra_files should extend scope: %v", j.Scope.Allowed)
	}
}

func TestComputeContract_AllCancelledNotDone(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	_ = dir
	l := store.NewLayout(root)
	eng := state.New(l, sid)
	if err := eng.CreateTask("o", model.Task{TaskID: "T-1", Title: "x", Acceptance: []string{"a"}, JobIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	if err := eng.CreateJob("c", model.Job{
		JobID: "J-1", TaskID: "T-1", CreatedBy: "c", TargetRuntime: model.RuntimeClaude,
		Role: model.RoleImplementation, Writes: true, Mode: model.ModeWorktree,
		StateRoot: l.StateRoot, RepoRoot: root, Workdir: root, Scope: model.Scope{Allowed: []string{"src/**"}},
		Delegation: model.Delegation{Depth: 1, ChainFingerprints: []string{"T-1:x"}},
		Budget:     model.JobBudget{TimeoutS: 1}, ResultContract: "job-result.schema.json",
	}); err != nil {
		t.Fatal(err)
	}
	_, _ = eng.TransitionJob("a", "J-1", 1, model.JobRunning)
	_, _ = eng.TransitionJob("a", "J-1", 2, model.JobFailed)
	_, _ = eng.ResolveJob("h", "J-1", model.JobCancelled)

	// only a cancelled job -> nothing actually done
	if c, _ := ComputeContract(l, sid, "T-1"); c.AllJobsDone {
		t.Fatal("a task with only cancelled jobs must not be AllJobsDone")
	}
}
