package cli

import (
	"fmt"

	"github.com/fengxd1222/multi-ai-workflow/internal/model"
	"github.com/fengxd1222/multi-ai-workflow/internal/state"
	"github.com/fengxd1222/multi-ai-workflow/internal/store"
)

// TaskCreate creates a task (rev3 §16). budget=0 means no token cap.
func TaskCreate(dir, sid, title string, accept []string, budget int) (string, error) {
	_, l, sid, err := sessionLayout(dir, sid)
	if err != nil {
		return "", err
	}
	if title == "" {
		return "", coded(ExitUsage, "task create needs --title")
	}
	if len(accept) == 0 {
		accept = []string{"work completed"}
	}
	tid := newTaskID()
	t := model.Task{
		TaskID: tid, Title: title, Status: model.TaskActive, Phase: "intake",
		Acceptance: accept, JobIDs: []string{},
	}
	if budget > 0 {
		t.Budget = &model.Budget{MaxTokens: budget}
	}
	if err := state.New(l, sid).CreateTask("orchestrator", t); err != nil {
		return "", err
	}
	fmt.Printf("created task %s: %s\n", tid, title)
	return tid, nil
}

// TaskPhase advances a task's phase via CAS-checked legality (rev3 §6).
func TaskPhase(dir, sid, taskID, to string) error {
	_, l, sid, err := sessionLayout(dir, sid)
	if err != nil {
		return err
	}
	var t model.Task
	if err := store.ReadJSON(l.TaskView(sid, taskID), &t); err != nil {
		return coded(ExitUsage, "task %s not found", taskID)
	}
	nt, err := state.New(l, sid).TransitionTaskPhase("orchestrator", taskID, t.Rev, to)
	if err != nil {
		return coded(ExitBlockedPolicy, "phase %s -> %s: %v", t.Phase, to, err)
	}
	fmt.Printf("task %s phase -> %s\n", taskID, nt.Phase)
	return nil
}
