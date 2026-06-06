package cli

import (
	"testing"

	"github.com/fengxd1222/multi-ai-workflow/internal/model"
	"github.com/fengxd1222/multi-ai-workflow/internal/state"
	"github.com/fengxd1222/multi-ai-workflow/internal/store"
)

func openTestGate(t *testing.T, l store.Layout, sid, gateID, jobID string) {
	t.Helper()
	g := model.Gate{
		GateID: gateID, TaskID: "T-1", JobID: jobID, Reason: "scope-violation",
		Options: []string{"approve_extra_files", "reject_and_rollback"}, Status: "open", CreatedAt: "t0",
	}
	if err := state.New(l, sid).OpenGate("o", g); err != nil {
		t.Fatal(err)
	}
}

func TestGate_ApproveExtendsScope(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	l := store.NewLayout(root)
	eng := state.New(l, sid)
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
	openTestGate(t, l, sid, "G-1", "J-1")

	if err := GateApprove(dir, sid, "G-1", "approve_extra_files", []string{"docs/**"}); err != nil {
		t.Fatal(err)
	}

	var j model.Job
	_ = store.ReadJSON(l.JobView(sid, "J-1"), &j)
	if !sliceHas(j.Scope.Allowed, "docs/**") {
		t.Fatalf("approve_extra_files did not extend scope: %v", j.Scope.Allowed)
	}
	var g model.Gate
	_ = store.ReadJSON(l.GateView(sid, "G-1"), &g)
	if g.Status != "approved" {
		t.Fatalf("gate status = %s want approved", g.Status)
	}
}

func TestGate_ListAndReject(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	l := store.NewLayout(root)
	openTestGate(t, l, sid, "G-1", "J-1")

	if err := GateList(dir, sid); err != nil {
		t.Fatal(err)
	}
	if err := GateShow(dir, sid, "G-1"); err != nil {
		t.Fatal(err)
	}
	if err := GateReject(dir, sid, "G-1"); err != nil {
		t.Fatal(err)
	}
	var g model.Gate
	_ = store.ReadJSON(l.GateView(sid, "G-1"), &g)
	if g.Status != "rejected" {
		t.Fatalf("gate status = %s want rejected", g.Status)
	}
}

func sliceHas(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
