package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fengxudong/harness/internal/model"
	"github.com/fengxudong/harness/internal/state"
	"github.com/fengxudong/harness/internal/store"
)

func TestGuardPretool_JobNotFound(t *testing.T) {
	dir, sid, _ := initSessionWithCommit(t)
	var out bytes.Buffer
	if err := GuardPretool(dir, sid, "J-none", "claude", strings.NewReader("{}"), &out); CodeOf(err) != ExitUsage {
		t.Fatalf("want ExitUsage, got %v", err)
	}
}

func TestGuardPosttool_ReadOnlyJobNoop(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	eng := state.New(store.NewLayout(root), sid)
	if err := eng.CreateJob("c", model.Job{
		JobID: "J-2", TaskID: "T-1", CreatedBy: "c", TargetRuntime: model.RuntimeClaude,
		Role: model.RoleAnalysis, Writes: false, Mode: model.ModeShared,
		StateRoot: root + "/.harness", RepoRoot: root, Workdir: root,
		Scope:      model.Scope{Allowed: []string{"src/**"}},
		Delegation: model.Delegation{Depth: 1, ChainFingerprints: []string{"T-1:x"}},
		Budget:     model.JobBudget{TimeoutS: 1}, ResultContract: "job-result.schema.json",
	}); err != nil {
		t.Fatal(err)
	}
	if err := GuardPosttool(dir, sid, "J-2"); err != nil {
		t.Fatalf("read-only posttool should be a no-op: %v", err)
	}
}

func TestComputeContract_OpenGateBlocks(t *testing.T) {
	_, sid, root := initSessionWithCommit(t)
	l := store.NewLayout(root)
	eng := state.New(l, sid)
	if err := eng.CreateTask("o", model.Task{TaskID: "T-1", Title: "x", Acceptance: []string{"a"}, JobIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	g := model.Gate{
		GateID: "G-1", TaskID: "T-1", JobID: "J-1", Reason: "x",
		Options: []string{"reject_and_rollback"}, Status: "open", CreatedAt: "t",
	}
	if err := store.WriteAtomicJSON(l.GateView(sid, "G-1"), g); err != nil {
		t.Fatal(err)
	}
	c, err := ComputeContract(l, sid, "T-1")
	if err != nil {
		t.Fatal(err)
	}
	if c.OpenGates != 1 || c.Satisfied() {
		t.Fatalf("open gate should block: %+v missing=%s", c, c.Missing())
	}
}

func TestTaskStop_OrchestratorMissingTaskArg(t *testing.T) {
	dir, sid, _ := initSessionWithCommit(t)
	if err := HookTaskStop(dir, sid, "orchestrator", "", ""); CodeOf(err) != ExitUsage {
		t.Fatalf("missing --task should be ExitUsage, got %v", err)
	}
}
