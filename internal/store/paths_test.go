package store

import (
	"strings"
	"testing"
)

func TestLayout_Paths(t *testing.T) {
	l := NewLayout("/repo")
	if l.StateRoot != "/repo/.harness" {
		t.Fatalf("state root: %s", l.StateRoot)
	}
	cases := map[string]string{
		l.Schemas():              "/repo/.harness/schemas",
		l.Reserved():             "/repo/.harness/reserved.json",
		l.Contract():             "/repo/.harness/workflow-contract.md",
		l.StateLock():            "/repo/.harness/current/state.lock",
		l.RecoverLock():          "/repo/.harness/current/recover.lock",
		l.StateSummary():         "/repo/.harness/current/state-summary.md",
		l.SessionFile("S-1"):     "/repo/.harness/sessions/S-1/session.json",
		l.SessionBaseline("S-1"): "/repo/.harness/sessions/S-1/session-baseline.json",
		l.ActiveTask("S-1"):      "/repo/.harness/sessions/S-1/active-task.json",
		l.EventFile("S-1", "a"):  "/repo/.harness/sessions/S-1/events/a.jsonl",
		l.JobView("S-1", "J-1"):  "/repo/.harness/sessions/S-1/views/jobs/J-1.json",
		l.TaskView("S-1", "T-1"): "/repo/.harness/sessions/S-1/views/tasks/T-1.json",
		l.GateView("S-1", "G-1"): "/repo/.harness/sessions/S-1/views/gates/G-1.json",
		l.Artifacts("S-1", "J"):  "/repo/.harness/sessions/S-1/artifacts/J",
		l.Trash("J-1"):           "/repo/.harness/.trash/J-1",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("path = %s want %s", got, want)
		}
	}
	if !strings.HasSuffix(l.Events("S-1"), "/events") {
		t.Errorf("events dir: %s", l.Events("S-1"))
	}
}

func TestWriteAtomicJSON_MarshalError(t *testing.T) {
	// channels are not JSON-serializable -> marshal error path.
	err := WriteAtomicJSON(t.TempDir()+"/x.json", map[string]any{"c": make(chan int)})
	if err == nil {
		t.Fatal("expected marshal error")
	}
}
