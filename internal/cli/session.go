package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fengxd1222/multi-ai-workflow/internal/event"
	"github.com/fengxd1222/multi-ai-workflow/internal/model"
	"github.com/fengxd1222/multi-ai-workflow/internal/store"
)

type baseline struct {
	HEAD       string   `json:"head"`
	Porcelain  []string `json:"porcelain"`
	CapturedAt string   `json:"captured_at"`
}

// SessionStart creates a new session: captures the repo baseline, writes the
// session + per-session active-task view, and records session.started
// (rev3 §2, §16). active-task is per-session, never a global slot (N33).
func SessionStart(dir string) (string, error) {
	root, err := repoRoot(dir)
	if err != nil {
		return "", coded(ExitUsage, "%s is not inside a git repository", dir)
	}
	l := store.NewLayout(root)
	if _, err := os.Stat(l.Schemas()); err != nil {
		return "", coded(ExitUsage, "harness not initialized in %s; run `harness init`", root)
	}

	sid := newSessionID()
	if err := os.MkdirAll(l.Session(sid), 0o755); err != nil {
		return "", err
	}

	sess := model.Session{
		SessionID:       sid,
		ProtocolVersion: model.ProtocolVersion,
		RepoRoot:        root,
		StateRoot:       l.StateRoot,
		CreatedAt:       event.Now(),
		Status:          "active",
		ActiveTaskID:    nil,
	}
	if err := store.WriteAtomicJSON(l.SessionFile(sid), sess); err != nil {
		return "", err
	}

	porc, err := porcelainBaseline(root)
	if err != nil {
		return "", err
	}
	bl := baseline{HEAD: headCommit(root), Porcelain: porc, CapturedAt: event.Now()}
	if err := store.WriteAtomicJSON(l.SessionBaseline(sid), bl); err != nil {
		return "", err
	}

	// per-session active task pointer (null until a task is created)
	if err := store.WriteAtomicJSON(l.ActiveTask(sid), map[string]any{"active_task_id": nil}); err != nil {
		return "", err
	}

	if err := appendSessionEvent(l, sid, "cli", model.EvSessionStarted, map[string]string{
		"session_id": sid, "repo_root": root,
	}); err != nil {
		return "", err
	}

	fmt.Printf("session %s started (repo %s)\n", sid, root)
	return sid, nil
}

func newSessionID() string { return genID("S") }

func appendSessionEvent(l store.Layout, sid, actor, typ string, payload any) error {
	p, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ev := model.Event{
		EventID: event.NewGenerator().New(),
		Actor:   actor,
		TS:      event.Now(),
		Type:    typ,
		Payload: p,
	}
	return event.AppendRaw(l, sid, ev)
}
