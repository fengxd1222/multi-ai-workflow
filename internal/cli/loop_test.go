package cli

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/fengxudong/harness/internal/adapter"
	"github.com/fengxudong/harness/internal/model"
	"github.com/fengxudong/harness/internal/runtime"
	"github.com/fengxudong/harness/internal/scope"
	"github.com/fengxudong/harness/internal/state"
	"github.com/fengxudong/harness/internal/store"
)

// TestClosedLoop drives a whole task from create -> delegate -> run (Mock worker)
// -> verify -> integrate -> handoff and asserts the completion contract is
// satisfied and a delivery branch exists. No real CLI is used (rev3 §19 M5).
func TestClosedLoop(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	l := store.NewLayout(root)

	tid, err := TaskCreate(dir, sid, "auth refactor", []string{"api unchanged"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	jobID, err := Delegate(dir, sid, DelegateSpec{
		TaskID: tid, Role: model.RoleImplementation, Runtime: model.RuntimeClaude,
		Allowed: []string{"src/**"}, Verify: []string{"true"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Run the job with a Mock worker that writes an in-scope file and returns completed.
	result := []byte(`{"job_id":"` + jobID + `","status":"completed","summary":"done"}`)
	a := &adapter.Adapter{
		L: l, SID: sid, Eng: state.New(l, sid),
		RT:       runtime.ScopeViolation(result, map[string]string{"src/new.ts": "export const y = 1\n"}),
		Reserved: scope.Reserved{}, BootID: "boot",
	}
	out, err := a.Run(context.Background(), jobID)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != model.JobCompleted {
		t.Fatalf("job not completed: %s (%s)", out.Status, out.Reason)
	}

	// task-level verify, integrate, handoff
	if err := VerifyTask(dir, sid, tid, root, []string{"true"}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := Integrate(dir, sid, tid); err != nil {
		t.Fatalf("integrate: %v", err)
	}
	if err := Handoff(dir, sid, tid); err != nil {
		t.Fatalf("handoff: %v", err)
	}

	// completion contract satisfied + orchestrator stop allowed
	c, err := ComputeContract(l, sid, tid)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Satisfied() {
		t.Fatalf("contract not satisfied: %+v missing=%s", c, c.Missing())
	}
	if err := HookTaskStop(dir, sid, "orchestrator", "", tid); err != nil {
		t.Fatalf("orchestrator stop should pass: %v", err)
	}

	// delivery branch exists and carries the change
	branch := "harness/integration/" + tid
	ls, _ := exec.Command("git", "-C", root, "ls-tree", "-r", "--name-only", branch).Output()
	if !strings.Contains(string(ls), "src/new.ts") {
		t.Fatalf("delivery branch %s missing src/new.ts:\n%s", branch, ls)
	}
}
