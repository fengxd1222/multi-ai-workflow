package cli

import (
	"os"
	"testing"

	"github.com/fengxudong/harness/internal/model"
	"github.com/fengxudong/harness/internal/state"
	"github.com/fengxudong/harness/internal/store"
)

func TestDelegate_Validation(t *testing.T) {
	dir, sid, _ := initSessionWithCommit(t)
	if _, err := Delegate(dir, sid, DelegateSpec{TaskID: "T-1", Runtime: "claude"}); CodeOf(err) != ExitUsage {
		t.Fatalf("missing role should be ExitUsage, got %v", err)
	}
	if _, err := Delegate(dir, sid, DelegateSpec{TaskID: "T-1", Role: "implementation", Runtime: "claude", Depth: 5}); CodeOf(err) != ExitDelegationLoop {
		t.Fatalf("excess depth should be ExitDelegationLoop, got %v", err)
	}
}

func TestDelegate_CreatesConsumableJob(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	jobID, err := Delegate(dir, sid, DelegateSpec{
		TaskID: "T-1", Role: model.RoleImplementation, Runtime: model.RuntimeClaude, Allowed: []string{"src/**"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var j model.Job
	if err := store.ReadJSON(store.NewLayout(root).JobView(sid, jobID), &j); err != nil {
		t.Fatal(err)
	}
	if j.Status != model.JobCreated || !j.Writes || j.Mode != model.ModeWorktree {
		t.Fatalf("delegated job wrong: %+v", j)
	}
}

func TestVerifyTask_Fails(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	if err := VerifyTask(dir, sid, "T-1", root, []string{"false"}); CodeOf(err) != ExitVerifyFailed {
		t.Fatalf("failing verify should be ExitVerifyFailed, got %v", err)
	}
}

func TestTask_CreateAndPhase(t *testing.T) {
	dir, sid, _ := initSessionWithCommit(t)
	if _, err := TaskCreate(dir, sid, "", nil, 0); CodeOf(err) != ExitUsage {
		t.Fatalf("missing title should be ExitUsage, got %v", err)
	}
	tid, err := TaskCreate(dir, sid, "x", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := TaskPhase(dir, sid, tid, "research"); err != nil {
		t.Fatalf("intake->research: %v", err)
	}
	if err := TaskPhase(dir, sid, tid, "handoff"); CodeOf(err) != ExitBlockedPolicy {
		t.Fatalf("illegal phase should be ExitBlockedPolicy, got %v", err)
	}
}

func TestIntegrate_DeniedOpensGate(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	l, workdir := runningWriteJob(t, sid, root) // J-1, denies package.json, running(rev2)

	// worker commits a denied change onto the job branch
	if err := os.WriteFile(workdir+"/package.json", []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, workdir, "git", "add", "-A")
	run(t, workdir, "git", "-c", "user.email=h@h", "-c", "user.name=h", "commit", "-qm", "bad")
	if _, err := state.New(l, sid).TransitionJob("a", "J-1", 2, model.JobCompleted); err != nil {
		t.Fatal(err)
	}

	if err := Integrate(dir, sid, "T-1"); CodeOf(err) != ExitNeedsHuman {
		t.Fatalf("denied integrate should be ExitNeedsHuman, got %v", err)
	}
	if entries, _ := os.ReadDir(l.GatesDir(sid)); len(entries) == 0 {
		t.Fatal("denied integrate should open a gate")
	}
}
