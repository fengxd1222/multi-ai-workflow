package state

import (
	"testing"

	"github.com/fengxd1222/multi-ai-workflow/internal/model"
)

func toNeedsHuman(t *testing.T, e *Engine, id string) {
	t.Helper()
	if err := e.CreateJob("c", minimalJob(id)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.TransitionJob("a", id, 1, model.JobRunning); err != nil {
		t.Fatal(err)
	}
	if _, err := e.TransitionJob("a", id, 2, model.JobNeedsHuman); err != nil {
		t.Fatal(err)
	}
}

func TestResolveJob_ReopenAndCancelRules(t *testing.T) {
	e := newEngine(t)
	toNeedsHuman(t, e, "J-1")

	// needs-human -> created (re-dispatch)
	j, err := e.ResolveJob("h", "J-1", model.JobCreated)
	if err != nil || j.Status != model.JobCreated {
		t.Fatalf("reopen: %+v err=%v", j, err)
	}
	// created is NOT cancellable
	if _, err := e.ResolveJob("h", "J-1", model.JobCancelled); err == nil {
		t.Fatal("created job should not be cancellable")
	}
}

func TestResolveJob_CancelFromCompleted(t *testing.T) {
	e := newEngine(t)
	if err := e.CreateJob("c", minimalJob("J-1")); err != nil {
		t.Fatal(err)
	}
	_, _ = e.TransitionJob("a", "J-1", 1, model.JobRunning)
	_, _ = e.TransitionJob("a", "J-1", 2, model.JobCompleted)
	// integrate-gate reject can cancel a completed-but-unintegratable job
	j, err := e.ResolveJob("h", "J-1", model.JobCancelled)
	if err != nil || j.Status != model.JobCancelled {
		t.Fatalf("cancel-from-completed: %+v err=%v", j, err)
	}
	// completed is NOT re-dispatchable to created
	if err := e.CreateJob("c", minimalJob("J-2")); err != nil {
		t.Fatal(err)
	}
	_, _ = e.TransitionJob("a", "J-2", 1, model.JobRunning)
	_, _ = e.TransitionJob("a", "J-2", 2, model.JobCompleted)
	if _, err := e.ResolveJob("h", "J-2", model.JobCreated); err == nil {
		t.Fatal("completed job should not be re-dispatchable")
	}
}

func TestResolveJob_InvalidFromRunning(t *testing.T) {
	e := newEngine(t)
	if err := e.CreateJob("c", minimalJob("J-1")); err != nil {
		t.Fatal(err)
	}
	_, _ = e.TransitionJob("a", "J-1", 1, model.JobRunning)
	if _, err := e.ResolveJob("h", "J-1", model.JobCancelled); err == nil {
		t.Fatal("running job should not be resolvable")
	}
}

func TestOpenGateForJob_Idempotent(t *testing.T) {
	e := newEngine(t)
	g1, err := e.OpenGateForJob("a", "T-1", "J-1", "scope-violation", nil)
	if err != nil {
		t.Fatal(err)
	}
	g2, _ := e.OpenGateForJob("a", "T-1", "J-1", "scope-violation", nil)
	if g1.GateID != g2.GateID {
		t.Fatalf("duplicate gate for same job+reason: %s vs %s", g1.GateID, g2.GateID)
	}
	// a different reason opens a distinct gate
	g3, _ := e.OpenGateForJob("a", "T-1", "J-1", "merge-conflict", nil)
	if g3.GateID == g1.GateID {
		t.Fatal("different reason should open a distinct gate")
	}
}
