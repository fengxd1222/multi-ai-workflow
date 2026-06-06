package event

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fengxd1222/multi-ai-workflow/internal/model"
	"github.com/fengxd1222/multi-ai-workflow/internal/store"
)

// Now returns an RFC3339 UTC timestamp with millisecond precision, used as the
// primary sort key for event folding.
func Now() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

// AppendRaw appends a fully-populated event as one line to
// events/<actor>.jsonl. The record + '\n' is written in a single write() under
// O_APPEND and fsync'd, so concurrent same-actor writers never interleave and a
// crash leaves at most a torn tail (rev3 §4; torn handled in Fold, N12).
func AppendRaw(l store.Layout, sid string, ev model.Event) error {
	if err := os.MkdirAll(l.Events(sid), 0o755); err != nil {
		return err
	}
	line, err := json.Marshal(ev) // compact: no interior newlines
	if err != nil {
		return err
	}
	line = append(line, '\n')

	f, err := os.OpenFile(l.EventFile(sid, ev.Actor), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(line); err != nil { // single write() call
		return err
	}
	return f.Sync()
}

// Fold reads every events/<actor>.jsonl in the session, tolerates a torn final
// line per file, and returns the events in deterministic total order
// (ts, actor, event_id) — the basis for view rebuild (rev3 §4, §15).
func Fold(l store.Layout, sid string) ([]model.Event, error) {
	entries, err := os.ReadDir(l.Events(sid))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var all []model.Event
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		evs, err := foldFile(filepath.Join(l.Events(sid), e.Name()))
		if err != nil {
			return nil, err
		}
		all = append(all, evs...)
	}

	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if a.TS != b.TS {
			return a.TS < b.TS
		}
		if a.Actor != b.Actor {
			return a.Actor < b.Actor
		}
		return a.EventID < b.EventID
	})
	return all, nil
}

// foldFile parses one event file. A line terminated by '\n' is authoritative:
// a parse failure there is real corruption (state-corrupt). A trailing segment
// with no terminating '\n' is a torn write from a crash and is tolerated
// (skipped) — rev3 N12.
func foldFile(path string) ([]model.Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var evs []model.Event
	for i := 0; i < len(data); {
		nl := bytes.IndexByte(data[i:], '\n')
		if nl < 0 {
			break // torn tail: incomplete final record, tolerate
		}
		line := bytes.TrimSpace(data[i : i+nl])
		i += nl + 1
		if len(line) == 0 {
			continue
		}
		var ev model.Event
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, fmt.Errorf("%s: corrupt event line: %w", filepath.Base(path), err)
		}
		evs = append(evs, ev)
	}
	return evs, nil
}
