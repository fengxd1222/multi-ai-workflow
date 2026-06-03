package state

import (
	"encoding/json"
	"fmt"

	"github.com/fengxudong/harness/internal/event"
	"github.com/fengxudong/harness/internal/model"
	"github.com/fengxudong/harness/internal/store"
)

// Engine performs serialized, CAS-checked state transitions for one session.
type Engine struct {
	L   store.Layout
	SID string
	gen *event.Generator
}

// New returns an Engine for the given session.
func New(l store.Layout, sid string) *Engine {
	return &Engine{L: l, SID: sid, gen: event.NewGenerator()}
}

// legal transition tables (rev3 §5 job, §6 task phase).
var jobLegal = map[string]map[string]bool{
	model.JobCreated: {model.JobRunning: true},
	model.JobRunning: {
		model.JobCompleted: true, model.JobFailed: true,
		model.JobNeedsHuman: true, model.JobTimeout: true,
	},
}

var phaseLegal = map[string]map[string]bool{
	"intake":    {"research": true},
	"research":  {"plan": true},
	"plan":      {"delegate": true},
	"delegate":  {"implement": true},
	"implement": {"verify": true},
	"verify":    {"integrate": true, "implement": true}, // implement = followup
	"integrate": {"handoff": true},
	"handoff":   {"done": true},
}

// CreateJob appends job.created (rev=1, status=created) and writes the view.
func (e *Engine) CreateJob(actor string, j model.Job) error {
	lk, err := store.AcquireLock(e.L.StateLock())
	if err != nil {
		return err
	}
	defer lk.Release()

	st, err := e.fold()
	if err != nil {
		return err
	}
	if _, exists := st.Jobs[j.JobID]; exists {
		return ErrExists
	}
	j.Rev = 1
	j.Status = model.JobCreated
	payload, err := json.Marshal(j)
	if err != nil {
		return err
	}
	ev := e.newEvent(actor, model.EvJobCreated, nil, payload)
	if err := event.AppendRaw(e.L, e.SID, ev); err != nil {
		return err
	}
	return store.WriteAtomicJSON(e.L.JobView(e.SID, j.JobID), j)
}

// TransitionJob moves a job to `to` if rev matches (CAS) and the transition is
// legal. Returns ErrCASRetry on rev mismatch (code 32) and ErrIllegalTransition
// (after recording policy.violation) for an illegal move.
func (e *Engine) TransitionJob(actor, jobID string, expectRev int, to string) (model.Job, error) {
	lk, err := store.AcquireLock(e.L.StateLock())
	if err != nil {
		return model.Job{}, err
	}
	defer lk.Release()

	st, err := e.fold()
	if err != nil {
		return model.Job{}, err
	}
	j := st.Jobs[jobID]
	if j == nil {
		return model.Job{}, ErrNotFound
	}
	if j.Rev != expectRev {
		return *j, ErrCASRetry
	}
	if !jobLegal[j.Status][to] {
		_ = e.appendPolicyViolation(actor, "job:"+jobID,
			fmt.Sprintf("illegal job transition %s -> %s", j.Status, to))
		return *j, ErrIllegalTransition
	}

	newRev := j.Rev + 1
	payload, _ := json.Marshal(statusChange{JobID: jobID, From: j.Status, To: to})
	cas := &model.CAS{Entity: "job:" + jobID, ExpectRev: expectRev, NewRev: newRev}
	ev := e.newEvent(actor, model.EvJobStatusChanged, cas, payload)
	if err := event.AppendRaw(e.L, e.SID, ev); err != nil {
		return *j, err
	}
	j.Status = to
	j.Rev = newRev
	if err := store.WriteAtomicJSON(e.L.JobView(e.SID, jobID), j); err != nil {
		return *j, err
	}
	return *j, nil
}

// CreateTask appends task.created and writes the view.
func (e *Engine) CreateTask(actor string, t model.Task) error {
	lk, err := store.AcquireLock(e.L.StateLock())
	if err != nil {
		return err
	}
	defer lk.Release()

	st, err := e.fold()
	if err != nil {
		return err
	}
	if _, exists := st.Tasks[t.TaskID]; exists {
		return ErrExists
	}
	t.Rev = 1
	if t.Status == "" {
		t.Status = model.TaskActive
	}
	if t.Phase == "" {
		t.Phase = "intake"
	}
	payload, err := json.Marshal(t)
	if err != nil {
		return err
	}
	ev := e.newEvent(actor, model.EvTaskCreated, nil, payload)
	if err := event.AppendRaw(e.L, e.SID, ev); err != nil {
		return err
	}
	return store.WriteAtomicJSON(e.L.TaskView(e.SID, t.TaskID), t)
}

// TransitionTaskPhase advances a task's phase with CAS + legality (rev3 §6 N22).
func (e *Engine) TransitionTaskPhase(actor, taskID string, expectRev int, to string) (model.Task, error) {
	lk, err := store.AcquireLock(e.L.StateLock())
	if err != nil {
		return model.Task{}, err
	}
	defer lk.Release()

	st, err := e.fold()
	if err != nil {
		return model.Task{}, err
	}
	t := st.Tasks[taskID]
	if t == nil {
		return model.Task{}, ErrNotFound
	}
	if t.Rev != expectRev {
		return *t, ErrCASRetry
	}
	if !phaseLegal[t.Phase][to] {
		_ = e.appendPolicyViolation(actor, "task:"+taskID,
			fmt.Sprintf("illegal phase transition %s -> %s", t.Phase, to))
		return *t, ErrIllegalTransition
	}

	newRev := t.Rev + 1
	payload, _ := json.Marshal(statusChange{TaskID: taskID, From: t.Phase, To: to})
	cas := &model.CAS{Entity: "task:" + taskID, ExpectRev: expectRev, NewRev: newRev}
	ev := e.newEvent(actor, model.EvTaskPhaseChanged, cas, payload)
	if err := event.AppendRaw(e.L, e.SID, ev); err != nil {
		return *t, err
	}
	t.Phase = to
	t.Rev = newRev
	if err := store.WriteAtomicJSON(e.L.TaskView(e.SID, taskID), t); err != nil {
		return *t, err
	}
	return *t, nil
}

// RebuildViews replays all events and rewrites every entity view. Idempotent;
// used by `harness recover` (rev3 §15). Equal to incremental views because both
// derive from Reduce.
func (e *Engine) RebuildViews() (jobs, tasks int, err error) {
	lk, err := store.AcquireLock(e.L.StateLock())
	if err != nil {
		return 0, 0, err
	}
	defer lk.Release()

	st, err := e.fold()
	if err != nil {
		return 0, 0, err
	}
	for _, j := range st.Jobs {
		if err := store.WriteAtomicJSON(e.L.JobView(e.SID, j.JobID), j); err != nil {
			return 0, 0, err
		}
	}
	for _, t := range st.Tasks {
		if err := store.WriteAtomicJSON(e.L.TaskView(e.SID, t.TaskID), t); err != nil {
			return 0, 0, err
		}
	}
	return len(st.Jobs), len(st.Tasks), nil
}

func (e *Engine) fold() (*State, error) {
	evs, err := event.Fold(e.L, e.SID)
	if err != nil {
		return nil, err
	}
	return Reduce(evs)
}

func (e *Engine) appendPolicyViolation(actor, entity, msg string) error {
	payload, _ := json.Marshal(map[string]string{"entity": entity, "message": msg})
	ev := e.newEvent(actor, model.EvPolicyViolation, nil, payload)
	return event.AppendRaw(e.L, e.SID, ev)
}

func (e *Engine) newEvent(actor, typ string, cas *model.CAS, payload json.RawMessage) model.Event {
	return model.Event{
		EventID: e.gen.New(),
		Actor:   actor,
		TS:      event.Now(),
		Type:    typ,
		CAS:     cas,
		Payload: payload,
	}
}
