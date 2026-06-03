package state

import (
	"errors"
	"testing"

	"github.com/fengxudong/harness/internal/model"
	"github.com/fengxudong/harness/internal/store"
)

func TestRebuildViews_IncludesTasks(t *testing.T) {
	e := newEngine(t)
	if err := e.CreateTask("o", model.Task{TaskID: "T-1", Title: "x", Acceptance: []string{"a"}, JobIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	if err := e.CreateJob("c", minimalJob("J-1")); err != nil {
		t.Fatal(err)
	}
	jobs, tasks, err := e.RebuildViews()
	if err != nil {
		t.Fatal(err)
	}
	if jobs != 1 || tasks != 1 {
		t.Fatalf("rebuild counts: jobs=%d tasks=%d want 1/1", jobs, tasks)
	}
	var tk model.Task
	if err := store.ReadJSON(e.L.TaskView(e.SID, "T-1"), &tk); err != nil {
		t.Fatalf("task view not rebuilt: %v", err)
	}
	if tk.TaskID != "T-1" || tk.Rev != 1 {
		t.Fatalf("rebuilt task wrong: %+v", tk)
	}
}

func TestJob_CreateDuplicate(t *testing.T) {
	e := newEngine(t)
	if err := e.CreateJob("a", minimalJob("J-1")); err != nil {
		t.Fatal(err)
	}
	if err := e.CreateJob("a", minimalJob("J-1")); !errors.Is(err, ErrExists) {
		t.Fatalf("want ErrExists, got %v", err)
	}
}

func TestJob_TransitionNotFound(t *testing.T) {
	e := newEngine(t)
	if _, err := e.TransitionJob("a", "J-missing", 1, model.JobRunning); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestTask_CreateDuplicate(t *testing.T) {
	e := newEngine(t)
	mk := func() model.Task {
		return model.Task{TaskID: "T-1", Title: "x", Acceptance: []string{"a"}, JobIDs: []string{}}
	}
	if err := e.CreateTask("o", mk()); err != nil {
		t.Fatal(err)
	}
	if err := e.CreateTask("o", mk()); !errors.Is(err, ErrExists) {
		t.Fatalf("want ErrExists, got %v", err)
	}
}

func TestTaskPhase_CASConflictAndFollowup(t *testing.T) {
	e := newEngine(t)
	if err := e.CreateTask("o", model.Task{TaskID: "T-1", Title: "x", Acceptance: []string{"a"}, JobIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	// wrong rev -> CAS retry
	if _, err := e.TransitionTaskPhase("o", "T-1", 99, "research"); !errors.Is(err, ErrCASRetry) {
		t.Fatalf("want ErrCASRetry, got %v", err)
	}
	// walk the legal chain to verify, then verify->implement (followup) must be legal
	steps := []struct {
		rev int
		to  string
	}{{1, "research"}, {2, "plan"}, {3, "delegate"}, {4, "implement"}, {5, "verify"}, {6, "implement"}}
	for _, s := range steps {
		if _, err := e.TransitionTaskPhase("o", "T-1", s.rev, s.to); err != nil {
			t.Fatalf("phase %d->%s: %v", s.rev, s.to, err)
		}
	}
	var tk model.Task
	if err := store.ReadJSON(e.L.TaskView(e.SID, "T-1"), &tk); err != nil {
		t.Fatal(err)
	}
	if tk.Phase != "implement" || tk.Rev != 7 {
		t.Fatalf("got phase=%s rev=%d want implement/7", tk.Phase, tk.Rev)
	}
}
