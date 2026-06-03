package event

import (
	"encoding/json"
	"os"
	"sync"
	"testing"

	"github.com/fengxudong/harness/internal/model"
	"github.com/fengxudong/harness/internal/store"
)

func newEvent(actor, typ, id, ts string) model.Event {
	return model.Event{EventID: id, Actor: actor, TS: ts, Type: typ, Payload: json.RawMessage(`{}`)}
}

func TestULID_MonotonicAndFormat(t *testing.T) {
	g := NewGenerator()
	prev := ""
	for i := 0; i < 5000; i++ {
		id := g.New()
		if len(id) != 26 {
			t.Fatalf("ulid len=%d want 26: %q", len(id), id)
		}
		if id <= prev {
			t.Fatalf("ulid not monotonic at %d: %q <= %q", i, id, prev)
		}
		prev = id
	}
}

func TestAppendFold_Roundtrip_TotalOrder(t *testing.T) {
	l := store.NewLayout(t.TempDir())
	sid := "S-1"
	// Two actors, interleaved ts; expect sort by (ts, actor, event_id).
	must(t, AppendRaw(l, sid, newEvent("claude", model.EvJobCreated, "01AA", "2026-06-03T10:00:00.002Z")))
	must(t, AppendRaw(l, sid, newEvent("codex", model.EvJobCreated, "01BB", "2026-06-03T10:00:00.001Z")))
	must(t, AppendRaw(l, sid, newEvent("claude", model.EvJobCreated, "01AC", "2026-06-03T10:00:00.001Z")))

	got, err := Fold(l, sid)
	if err != nil {
		t.Fatal(err)
	}
	order := []string{got[0].Actor + ":" + got[0].EventID, got[1].Actor + ":" + got[1].EventID, got[2].Actor + ":" + got[2].EventID}
	want := []string{"claude:01AC", "codex:01BB", "claude:01AA"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order[%d]=%s want %s (full %v)", i, order[i], want[i], order)
		}
	}
}

func TestFold_ToleratesTornTail(t *testing.T) {
	l := store.NewLayout(t.TempDir())
	sid := "S-1"
	must(t, AppendRaw(l, sid, newEvent("codex", model.EvJobCreated, "01AA", "2026-06-03T10:00:00.000Z")))
	// Simulate a crash mid-append: a second record written without trailing \n.
	f, _ := os.OpenFile(l.EventFile(sid, "codex"), os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString(`{"event_id":"01BB","actor":"codex","ts":"x","type":"job.created","payl`)
	f.Close()

	got, err := Fold(l, sid)
	if err != nil {
		t.Fatalf("torn tail must be tolerated, got err: %v", err)
	}
	if len(got) != 1 || got[0].EventID != "01AA" {
		t.Fatalf("expected only the complete event, got %d: %+v", len(got), got)
	}
}

func TestFold_RejectsMidFileCorruption(t *testing.T) {
	l := store.NewLayout(t.TempDir())
	sid := "S-1"
	// A corrupt line that IS terminated by \n (not a torn tail) -> real corruption.
	must(t, os.MkdirAll(l.Events(sid), 0o755))
	must(t, os.WriteFile(l.EventFile(sid, "codex"),
		[]byte("{not json}\n{\"event_id\":\"01BB\",\"actor\":\"codex\",\"ts\":\"t\",\"type\":\"x\",\"payload\":{}}\n"), 0o644))

	if _, err := Fold(l, sid); err == nil {
		t.Fatal("expected state-corrupt error for terminated bad line")
	}
}

func TestAppend_ConcurrentSameActor_NoLoss(t *testing.T) {
	l := store.NewLayout(t.TempDir())
	sid := "S-1"
	g := NewGenerator()
	const n = 200
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ev := newEvent("codex", model.EvUsageReported, g.New(), Now())
			if err := AppendRaw(l, sid, ev); err != nil {
				t.Errorf("append: %v", err)
			}
		}()
	}
	wg.Wait()

	got, err := Fold(l, sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != n {
		t.Fatalf("concurrent append lost events: got %d want %d", len(got), n)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
