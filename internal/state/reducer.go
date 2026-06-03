package state

import (
	"encoding/json"
	"fmt"

	"github.com/fengxudong/harness/internal/model"
)

// State is the reduced view of all entities, derived purely from folded events.
type State struct {
	Jobs  map[string]*model.Job
	Tasks map[string]*model.Task
}

// statusChange is the payload shape for *.status_changed / task.phase_changed.
// The optional worker/worktree fields let created->running also stamp the
// runtime identity and worktree binding in a single event (rev3 §8 step 4).
type statusChange struct {
	JobID      string        `json:"job_id,omitempty"`
	TaskID     string        `json:"task_id,omitempty"`
	From       string        `json:"from"`
	To         string        `json:"to"`
	Reason     string        `json:"reason,omitempty"`
	Worker     *model.Worker `json:"worker,omitempty"`
	Workdir    string        `json:"workdir,omitempty"`
	Branch     string        `json:"branch,omitempty"`
	BaseCommit string        `json:"base_commit,omitempty"`
}

// Reduce replays events in order and reconstructs entity state. This is the
// authoritative read path (events are truth); the same function backs both the
// in-line CAS check and full view rebuild, so incremental == rebuilt by
// construction (rev3 §15, N11).
func Reduce(evs []model.Event) (*State, error) {
	s := &State{Jobs: map[string]*model.Job{}, Tasks: map[string]*model.Task{}}
	for _, ev := range evs {
		switch ev.Type {
		case model.EvJobCreated:
			var j model.Job
			if err := json.Unmarshal(ev.Payload, &j); err != nil {
				return nil, fmt.Errorf("reduce job.created: %w", err)
			}
			s.Jobs[j.JobID] = &j

		case model.EvJobStatusChanged:
			var p statusChange
			if err := json.Unmarshal(ev.Payload, &p); err != nil {
				return nil, err
			}
			j := s.Jobs[p.JobID]
			if j == nil {
				return nil, fmt.Errorf("job.status_changed for unknown job %s", p.JobID)
			}
			j.Status = p.To
			if ev.CAS != nil {
				j.Rev = ev.CAS.NewRev
			}
			if p.Worker != nil {
				j.Worker = p.Worker
			}
			if p.Workdir != "" {
				j.Workdir = p.Workdir
			}
			if p.Branch != "" {
				b := p.Branch
				j.Branch = &b
			}
			if p.BaseCommit != "" {
				c := p.BaseCommit
				j.BaseCommit = &c
			}

		case model.EvTaskCreated:
			var t model.Task
			if err := json.Unmarshal(ev.Payload, &t); err != nil {
				return nil, fmt.Errorf("reduce task.created: %w", err)
			}
			s.Tasks[t.TaskID] = &t

		case model.EvTaskPhaseChanged, model.EvTaskStatusChange:
			var p statusChange
			if err := json.Unmarshal(ev.Payload, &p); err != nil {
				return nil, err
			}
			t := s.Tasks[p.TaskID]
			if t == nil {
				return nil, fmt.Errorf("%s for unknown task %s", ev.Type, p.TaskID)
			}
			if ev.Type == model.EvTaskPhaseChanged {
				t.Phase = p.To
			} else {
				t.Status = p.To
			}
			if ev.CAS != nil {
				t.Rev = ev.CAS.NewRev
			}
		}
		// Other event types (usage.reported, scope.violation, ...) do not change
		// reduced core entity state in M1.
	}
	return s, nil
}
