package cli

import (
	"os"
	"path/filepath"
	"strings"
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

func TestDelegate_FromAttachesFindings(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	l := store.NewLayout(root)

	// simulate a completed analysis job that produced findings
	if err := os.MkdirAll(filepath.Dir(l.Findings(sid, "J-analysis")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.Findings(sid, "J-analysis"), []byte("# Findings\nauth uses bcrypt\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	jobID, err := Delegate(dir, sid, DelegateSpec{
		TaskID: "T-1", Role: model.RoleImplementation, Runtime: model.RuntimeClaude,
		Allowed: []string{"src/**"}, From: []string{"J-analysis"},
		Context: []string{"src/auth/service.ts"}, Constraints: []string{"keep API"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var j model.Job
	if err := store.ReadJSON(l.JobView(sid, jobID), &j); err != nil {
		t.Fatal(err)
	}
	// findings ref (repo-relative) + explicit context both present
	hasFindings := false
	for _, r := range j.ContextRefs {
		if strings.Contains(r, "findings/J-analysis.md") {
			hasFindings = true
		}
	}
	if !hasFindings {
		t.Fatalf("--from did not attach findings ref: %v", j.ContextRefs)
	}
	if !sliceHas(j.ContextRefs, "src/auth/service.ts") {
		t.Fatalf("--context not attached: %v", j.ContextRefs)
	}
	if !sliceHas(j.Constraints, "keep API") {
		t.Fatalf("--constraint not attached: %v", j.Constraints)
	}
}

func TestDelegate_TrellisTaskEnriches(t *testing.T) {
	dir, sid, root := initSessionWithCommit(t)
	// lay down a Trellis task under the repo
	td := filepath.Join(root, ".trellis", "tasks", "02-27-login")
	if err := os.MkdirAll(td, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(td, "task.json"), []byte(`{"id":"02-27-login","title":"Add login validation","status":"planning","branch":"feat/login"}`), 0o644)
	os.WriteFile(filepath.Join(td, "prd.md"), []byte("# Login\nValidate inputs; empty -> E_BADPASS."), 0o644)
	os.WriteFile(filepath.Join(td, "implement.jsonl"), []byte(`{"file":"src/auth/service.js","reason":"target"}`+"\n"), 0o644)

	jobID, err := Delegate(dir, sid, DelegateSpec{
		TaskID: "T-1", Role: model.RoleImplementation, Runtime: model.RuntimeClaude,
		TrellisTask: "02-27-login", Allowed: []string{"src/**"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var j model.Job
	if err := store.ReadJSON(store.NewLayout(root).JobView(sid, jobID), &j); err != nil {
		t.Fatal(err)
	}
	if j.Goal != "Add login validation" {
		t.Errorf("goal not from trellis title: %q", j.Goal)
	}
	if !strings.Contains(j.Brief, "Validate inputs") {
		t.Errorf("brief not from prd: %q", j.Brief)
	}
	if !sliceHas(j.ContextRefs, "src/auth/service.js") {
		t.Errorf("context_refs not from implement.jsonl: %v", j.ContextRefs)
	}
	if j.TrellisTask != "02-27-login" {
		t.Errorf("trellis_task not recorded: %q", j.TrellisTask)
	}
}

func TestDelegate_TrellisTaskNoWorkspace(t *testing.T) {
	dir, sid, _ := initSessionWithCommit(t)
	if _, err := Delegate(dir, sid, DelegateSpec{
		TaskID: "T-1", Role: model.RoleImplementation, Runtime: model.RuntimeClaude,
		TrellisTask: "02-27-x",
	}); CodeOf(err) != ExitUsage {
		t.Fatalf("--trellis-task without .trellis should be ExitUsage, got %v", err)
	}
}

func TestDelegate_FromMissingFindings(t *testing.T) {
	dir, sid, _ := initSessionWithCommit(t)
	if _, err := Delegate(dir, sid, DelegateSpec{
		TaskID: "T-1", Role: model.RoleImplementation, Runtime: model.RuntimeClaude,
		From: []string{"J-nope"},
	}); CodeOf(err) != ExitUsage {
		t.Fatalf("--from with no findings should be ExitUsage, got %v", err)
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
