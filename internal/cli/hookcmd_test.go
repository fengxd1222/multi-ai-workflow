package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/fengxd1222/multi-ai-workflow/internal/model"
	"github.com/fengxd1222/multi-ai-workflow/internal/state"
	"github.com/fengxd1222/multi-ai-workflow/internal/store"
	"github.com/fengxd1222/multi-ai-workflow/internal/worktree"
)

func boolp(b bool) *bool { return &b }

// runningWriteJob creates J-1 (write/worktree), adds the worktree, and stamps it
// running. Returns layout, worktree workdir.
func runningWriteJob(t *testing.T, sid, root string) (store.Layout, string) {
	t.Helper()
	l := store.NewLayout(root)
	eng := state.New(l, sid)
	j := model.Job{
		JobID: "J-1", TaskID: "T-1", CreatedBy: "c", TargetRuntime: model.RuntimeClaude,
		Role: model.RoleImplementation, Writes: true, Mode: model.ModeWorktree,
		StateRoot: l.StateRoot, RepoRoot: root, Workdir: root,
		Scope:      model.Scope{Allowed: []string{"src/**"}, Denied: []string{"package.json"}},
		Delegation: model.Delegation{Depth: 1, ChainFingerprints: []string{"T-1:x"}},
		Budget:     model.JobBudget{TimeoutS: 2}, ResultContract: "job-result.schema.json",
	}
	if err := eng.CreateJob("c", j); err != nil {
		t.Fatal(err)
	}
	wt, err := worktree.Add(root, "J-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eng.TransitionJobRunning("a", "J-1", 1, &model.Worker{PID: 1, BootID: "b"}, wt.Workdir, wt.Branch, wt.BaseCommit); err != nil {
		t.Fatal(err)
	}
	return l, wt.Workdir
}

func TestGuardPretool_Decisions(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	runningWriteJob(t, sid, root)

	cases := []struct {
		payload string
		want    string // permissionDecision
	}{
		{`{"tool_name":"Bash","tool_input":{"command":"rm -rf x"}}`, "deny"},
		{`{"tool_name":"Write","tool_input":{"file_path":"src/x.ts"}}`, "allow"},
		{`{"tool_name":"Write","tool_input":{"file_path":"package.json"}}`, "deny"},
		{`{"tool_name":"Write","tool_input":{"file_path":"docs/r.md"}}`, "ask"},
		{`not json`, "ask"},
	}
	for _, c := range cases {
		var out bytes.Buffer
		if err := GuardPretool(dir, "", sid, "J-1", "claude", strings.NewReader(c.payload), &out); err != nil {
			t.Fatalf("payload %q: %v", c.payload, err)
		}
		if !strings.Contains(out.String(), `"permissionDecision":"`+c.want+`"`) {
			t.Errorf("payload %q -> %s, want decision %s", c.payload, out.String(), c.want)
		}
	}
}

func TestGuardPosttool_DetectsViolation(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	_, workdir := runningWriteJob(t, sid, root)
	// worker writes an out-of-scope file directly in the worktree
	if err := os.WriteFile(workdir+"/package.json", []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := GuardPosttool(dir, "", sid, "J-1")
	if CodeOf(err) != ExitBlockedPolicy {
		t.Fatalf("want ExitBlockedPolicy for out-of-scope write, got %v", err)
	}
}

func TestTaskStop_Worker(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	l, _ := runningWriteJob(t, sid, root)
	eng := state.New(l, sid)

	// still running -> blocked
	if err := HookTaskStop(dir, sid, "worker", "J-1", ""); CodeOf(err) != ExitBlockedPolicy {
		t.Fatalf("running worker stop should block, got %v", err)
	}
	// terminal but no result artifact -> blocked
	if _, err := eng.TransitionJob("a", "J-1", 2, model.JobCompleted); err != nil {
		t.Fatal(err)
	}
	if err := HookTaskStop(dir, sid, "worker", "J-1", ""); CodeOf(err) != ExitBlockedPolicy {
		t.Fatalf("no-result worker stop should block, got %v", err)
	}
	// write a result artifact -> ok
	if err := os.MkdirAll(l.Artifacts(sid, "J-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.FinalJSON(sid, "J-1"), []byte(`{"job_id":"J-1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := HookTaskStop(dir, sid, "worker", "J-1", ""); err != nil {
		t.Fatalf("worker stop with result should pass: %v", err)
	}
}

func TestTaskStop_OrchestratorContract(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	l := store.NewLayout(root)
	eng := state.New(l, sid)
	if err := eng.CreateTask("o", model.Task{TaskID: "T-1", Title: "x", Acceptance: []string{"a"}, JobIDs: []string{"J-1"}}); err != nil {
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

	// jobs incomplete, no verify/handoff -> blocked
	if err := HookTaskStop(dir, sid, "orchestrator", "", "T-1"); CodeOf(err) != ExitBlockedPolicy {
		t.Fatalf("incomplete contract should block, got %v", err)
	}

	// satisfy all four conditions
	if _, err := eng.TransitionJob("a", "J-1", 1, model.JobRunning); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.TransitionJob("a", "J-1", 2, model.JobCompleted); err != nil {
		t.Fatal(err)
	}
	v := model.Verification{Level: "task", Checks: []model.VerificationCheck{
		{Command: "go test", Result: "passed", Required: boolp(true)},
	}}
	if err := store.WriteAtomicJSON(l.Verification(sid, "T-1"), v); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.Handoff(sid, "T-1"), []byte("# handoff\ndone\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := HookTaskStop(dir, sid, "orchestrator", "", "T-1"); err != nil {
		t.Fatalf("satisfied contract should pass: %v", err)
	}
}

func TestTaskStop_BadRole(t *testing.T) {
	dir, sid, _ := initSessionWithCommit(t)
	if err := HookTaskStop(dir, sid, "bogus", "", ""); CodeOf(err) != ExitUsage {
		t.Fatalf("bad role should be ExitUsage, got %v", err)
	}
}
