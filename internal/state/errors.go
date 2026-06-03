// Package state implements the transition protocol over the event log: under
// state.lock it folds events to the authoritative entity state, checks CAS and
// transition legality, appends the event, then refreshes the materialized view
// (rev3 §4, §5, §6). Views are caches; events are truth (N11).
package state

import "errors"

var (
	// ErrCASRetry means the entity revision changed under us (exit code 32).
	ErrCASRetry = errors.New("cas mismatch (retry)")
	// ErrIllegalTransition means the requested transition is not in the table;
	// a policy.violation event is recorded.
	ErrIllegalTransition = errors.New("illegal transition")
	// ErrNotFound means the entity does not exist in the folded state.
	ErrNotFound = errors.New("entity not found")
	// ErrExists means a create collided with an existing entity.
	ErrExists = errors.New("entity already exists")
)
