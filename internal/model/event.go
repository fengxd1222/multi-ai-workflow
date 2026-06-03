// Package model defines the on-disk data contracts for the harness protocol.
// Structs mirror schemas/*.json (rev3 §3). events are the single source of
// truth; the entity structs (Session/Task/Job/Gate) are materialized views
// that can be rebuilt by folding events.
package model

import "encoding/json"

// Event is one line in events/<actor>.jsonl — the authoritative unit of state
// change (rev3 §4). See schemas/event.schema.json.
type Event struct {
	EventID  string          `json:"event_id"`
	Actor    string          `json:"actor"`
	TS       string          `json:"ts"`
	Type     string          `json:"type"`
	CausedBy *string         `json:"caused_by,omitempty"`
	CAS      *CAS            `json:"cas,omitempty"`
	Payload  json.RawMessage `json:"payload"`
}

// CAS is the compare-and-swap credential carried by every state-transition
// event. Under state.lock the writer checks entity.rev == ExpectRev before the
// append is allowed (rev3 §4, fixes N2/N4).
type CAS struct {
	Entity    string `json:"entity"`
	ExpectRev int    `json:"expect_rev"`
	NewRev    int    `json:"new_rev"`
}

// Event type constants. Keep in sync with schemas/event.schema.json enum.
const (
	EvSessionStarted   = "session.started"
	EvSessionEnded     = "session.ended"
	EvTaskCreated      = "task.created"
	EvTaskPhaseChanged = "task.phase_changed"
	EvTaskStatusChange = "task.status_changed"
	EvJobCreated       = "job.created"
	EvJobStatusChanged = "job.status_changed"
	EvJobScopeExtended = "job.scope_extended"
	EvWorktreeCreated  = "worktree.created"
	EvWorktreeRemoved  = "worktree.removed"
	EvUsageReported    = "usage.reported"
	EvVerifyCompleted  = "verify.completed"
	EvScopeViolation   = "scope.violation"
	EvPolicyViolation  = "policy.violation"
	EvGateOpened       = "gate.opened"
	EvGateResolved     = "gate.resolved"
	EvIntegrateDone    = "integrate.completed"
	EvHandoffWritten   = "handoff.written"
	EvRecoverRan       = "recover.ran"
)

// transitionTypes are events that MUST carry a CAS credential.
var transitionTypes = map[string]bool{
	EvJobStatusChanged: true,
	EvTaskPhaseChanged: true,
	EvTaskStatusChange: true,
}

// IsTransition reports whether the event type requires a CAS credential.
func (e Event) IsTransition() bool { return transitionTypes[e.Type] }
