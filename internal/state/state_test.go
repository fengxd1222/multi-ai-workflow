package state

import (
	"errors"
	"os"
	"testing"

	"github.com/fengxd1222/multi-ai-workflow/internal/event"
	"github.com/fengxd1222/multi-ai-workflow/internal/model"
	"github.com/fengxd1222/multi-ai-workflow/internal/store"
)

func minimalJob(id string) model.Job {
	return model.Job{
		JobID: id, TaskID: "T-1", CreatedBy: "codex-main",
		TargetRuntime: model.RuntimeClaude, Role: model.RoleImplementation,
		Writes: true, Mode: model.ModeWorktree,
		StateRoot: "/repo/.harness", RepoRoot: "/repo", Workdir: "/repo/.worktrees/" + id,
		Scope:          model.Scope{Allowed: []string{"src/**"}, Denied: []string{"package.json"}},
		Delegation:     model.Delegation{Depth: 1, ChainFingerprints: []string{"T-1:x"}},
		Budget:         model.JobBudget{MaxTokens: 1000, TimeoutS: 60},
		ResultContract: "job-result.schema.json",
	}
}

func newEngine(t *testing.T) *Engine {
	t.Helper()
	return New(store.NewLayout(t.TempDir()), "S-1")
}

func TestJob_CreateAndTransition(t *testing.T) {
	e := newEngine(t)
	if err := e.CreateJob("codex-main", minimalJob("J-1")); err != nil {
		t.Fatal(err)
	}
	j, err := e.TransitionJob("claude-impl", "J-1", 1, model.JobRunning)
	if err != nil {
		t.Fatalf("created->running: %v", err)
	}
	if j.Status != model.JobRunning || j.Rev != 2 {
		t.Fatalf("got status=%s rev=%d want running/2", j.Status, j.Rev)
	}
	j, err = e.TransitionJob("claude-impl", "J-1", 2, model.JobCompleted)
	if err != nil {
		t.Fatalf("running->completed: %v", err)
	}
	if j.Status != model.JobCompleted || j.Rev != 3 {
		t.Fatalf("got status=%s rev=%d want completed/3", j.Status, j.Rev)
	}

	// View on disk reflects the latest state.
	var view model.Job
	if err := store.ReadJSON(e.L.JobView("S-1", "J-1"), &view); err != nil {
		t.Fatal(err)
	}
	if view.Status != model.JobCompleted || view.Rev != 3 {
		t.Fatalf("view stale: %+v", view)
	}
}

func TestJob_CASConflict(t *testing.T) {
	e := newEngine(t)
	_ = e.CreateJob("codex-main", minimalJob("J-1"))
	// job is at rev 1; transitioning with the wrong expected rev must be rejected.
	if _, err := e.TransitionJob("claude", "J-1", 0, model.JobRunning); !errors.Is(err, ErrCASRetry) {
		t.Fatalf("want ErrCASRetry, got %v", err)
	}
	// State unchanged (still created/rev1).
	var view model.Job
	_ = store.ReadJSON(e.L.JobView("S-1", "J-1"), &view)
	if view.Status != model.JobCreated || view.Rev != 1 {
		t.Fatalf("CAS-rejected write must not mutate: %+v", view)
	}
}

func TestJob_IllegalTransitionRecordsPolicyViolation(t *testing.T) {
	e := newEngine(t)
	_ = e.CreateJob("codex-main", minimalJob("J-1"))
	// created -> completed skips running: illegal.
	if _, err := e.TransitionJob("claude", "J-1", 1, model.JobCompleted); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("want ErrIllegalTransition, got %v", err)
	}
	evs, _ := event.Fold(e.L, "S-1")
	found := false
	for _, ev := range evs {
		if ev.Type == model.EvPolicyViolation {
			found = true
		}
	}
	if !found {
		t.Fatal("illegal transition must record a policy.violation event")
	}
}

func TestRebuildViews_EqualsIncrementalAndIdempotent(t *testing.T) {
	e := newEngine(t)
	_ = e.CreateJob("codex-main", minimalJob("J-1"))
	_, _ = e.TransitionJob("claude", "J-1", 1, model.JobRunning)
	_, _ = e.TransitionJob("claude", "J-1", 2, model.JobNeedsHuman)

	incremental, err := os.ReadFile(e.L.JobView("S-1", "J-1"))
	if err != nil {
		t.Fatal(err)
	}

	// Wipe views, rebuild purely from events.
	if err := os.RemoveAll(e.L.Views("S-1")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.RebuildViews(); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := os.ReadFile(e.L.JobView("S-1", "J-1"))
	if err != nil {
		t.Fatalf("view not rebuilt: %v", err)
	}
	if string(incremental) != string(rebuilt) {
		t.Fatalf("rebuilt != incremental\n inc: %s\n reb: %s", incremental, rebuilt)
	}

	// Idempotent.
	if _, _, err := e.RebuildViews(); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(e.L.JobView("S-1", "J-1"))
	if string(again) != string(rebuilt) {
		t.Fatal("RebuildViews not idempotent")
	}
}

func TestTaskPhase_LegalAndIllegal(t *testing.T) {
	e := newEngine(t)
	_ = e.CreateTask("orchestrator", model.Task{
		TaskID: "T-1", Title: "x", Acceptance: []string{"a"}, JobIDs: []string{},
	})
	// intake -> research legal.
	if _, err := e.TransitionTaskPhase("orchestrator", "T-1", 1, "research"); err != nil {
		t.Fatalf("intake->research: %v", err)
	}
	// research -> handoff illegal (skips middle).
	if _, err := e.TransitionTaskPhase("orchestrator", "T-1", 2, "handoff"); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("want ErrIllegalTransition, got %v", err)
	}
}
