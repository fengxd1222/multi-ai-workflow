package state

import (
	"errors"
	"os"
	"testing"

	"github.com/fengxd1222/multi-ai-workflow/internal/model"
	"github.com/fengxd1222/multi-ai-workflow/internal/store"
)

func TestGate_OpenResolveRebuild(t *testing.T) {
	e := newEngine(t)
	g := model.Gate{
		GateID: "G-1", TaskID: "T-1", JobID: "J-1", Reason: "scope-violation",
		Options: []string{"reject_and_rollback"}, Status: "open", CreatedAt: "t0",
	}
	if err := e.OpenGate("o", g); err != nil {
		t.Fatal(err)
	}
	res := &model.Resolution{Option: "reject_and_rollback", ResolvedAt: "t1", By: "human"}
	ng, err := e.ResolveGate("human", "G-1", "rejected", res)
	if err != nil || ng.Status != "rejected" || ng.Resolution == nil {
		t.Fatalf("resolve: %+v err=%v", ng, err)
	}
	// rebuild from events preserves the resolved gate
	if err := os.RemoveAll(e.L.Views(e.SID)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.RebuildViews(); err != nil {
		t.Fatal(err)
	}
	var gv model.Gate
	if err := store.ReadJSON(e.L.GateView(e.SID, "G-1"), &gv); err != nil {
		t.Fatal(err)
	}
	if gv.Status != "rejected" {
		t.Fatalf("rebuilt gate status = %s want rejected", gv.Status)
	}
}

func TestExtendJobScope(t *testing.T) {
	e := newEngine(t)
	if err := e.CreateJob("c", minimalJob("J-1")); err != nil {
		t.Fatal(err)
	}
	j, err := e.ExtendJobScope("human", "J-1", 1, []string{"docs/**"})
	if err != nil {
		t.Fatal(err)
	}
	if j.Rev != 2 || !contains(j.Scope.Allowed, "docs/**") {
		t.Fatalf("scope not extended: rev=%d allowed=%v", j.Rev, j.Scope.Allowed)
	}
	// CAS conflict at the stale rev
	if _, err := e.ExtendJobScope("human", "J-1", 1, []string{"x/**"}); !errors.Is(err, ErrCASRetry) {
		t.Fatalf("want ErrCASRetry, got %v", err)
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
