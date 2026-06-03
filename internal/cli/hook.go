package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fengxudong/harness/internal/model"
	"github.com/fengxudong/harness/internal/store"
)

// HookTaskStop is the Stop / TaskCompleted gate (rev3 §6 §11). It is split by
// actor role (N21): a worker's stop only requires that it produced a result; an
// orchestrator's stop enforces the full task completion contract.
func HookTaskStop(dir, sid, role, jobID, taskID string) error {
	root, err := repoRoot(dir)
	if err != nil {
		return coded(ExitUsage, "%s is not inside a git repository", dir)
	}
	l := store.NewLayout(root)
	if sid == "" {
		if sid, err = latestSession(l); err != nil {
			return err
		}
	}

	switch role {
	case "worker":
		return workerStop(l, sid, jobID)
	case "orchestrator":
		return orchestratorStop(l, sid, taskID)
	default:
		return coded(ExitUsage, "task-stop needs --role worker|orchestrator")
	}
}

// workerStop blocks a worker session ending without a produced result.
func workerStop(l store.Layout, sid, jobID string) error {
	var job model.Job
	if err := store.ReadJSON(l.JobView(sid, jobID), &job); err != nil {
		return coded(ExitUsage, "job %s not found", jobID)
	}
	if !isTerminal(job.Status) {
		return coded(ExitBlockedPolicy, "worker stop blocked: job %s is %s (not terminal)", jobID, job.Status)
	}
	if fi, err := os.Stat(l.FinalJSON(sid, jobID)); err != nil || fi.Size() == 0 {
		return coded(ExitBlockedPolicy, "worker stop blocked: job %s has no result artifact", jobID)
	}
	fmt.Printf("worker stop ok: job %s = %s\n", jobID, job.Status)
	return nil
}

// orchestratorStop enforces the four-condition completion contract.
func orchestratorStop(l store.Layout, sid, taskID string) error {
	if taskID == "" {
		return coded(ExitUsage, "orchestrator task-stop needs --task")
	}
	c, err := ComputeContract(l, sid, taskID)
	if err != nil {
		return err
	}
	if c.Satisfied() {
		fmt.Printf("task %s completion contract satisfied\n", taskID)
		return nil
	}
	return coded(ExitBlockedPolicy, "task %s not done: %s", taskID, c.Missing())
}

// Contract is the machine-readable task completion contract (rev3 §6).
type Contract struct {
	AllJobsDone    bool
	VerifyPassed   bool
	HandoffWritten bool
	OpenGates      int
}

// Satisfied reports whether all four conditions hold.
func (c Contract) Satisfied() bool {
	return c.AllJobsDone && c.VerifyPassed && c.HandoffWritten && c.OpenGates == 0
}

// Missing lists the unmet conditions for a human-readable block reason.
func (c Contract) Missing() string {
	var m []string
	if !c.AllJobsDone {
		m = append(m, "jobs-incomplete")
	}
	if !c.VerifyPassed {
		m = append(m, "verify-not-passed")
	}
	if !c.HandoffWritten {
		m = append(m, "handoff-missing")
	}
	if c.OpenGates > 0 {
		m = append(m, fmt.Sprintf("open-gates=%d", c.OpenGates))
	}
	return strings.Join(m, ",")
}

// ComputeContract derives the contract from current state + on-disk artifacts.
func ComputeContract(l store.Layout, sid, taskID string) (Contract, error) {
	var task model.Task
	if err := store.ReadJSON(l.TaskView(sid, taskID), &task); err != nil {
		return Contract{}, coded(ExitUsage, "task %s not found", taskID)
	}

	c := Contract{AllJobsDone: true}
	for _, jid := range task.JobIDs {
		var j model.Job
		if err := store.ReadJSON(l.JobView(sid, jid), &j); err != nil || j.Status != model.JobCompleted {
			c.AllJobsDone = false
		}
	}

	var v model.Verification
	if err := store.ReadJSON(l.Verification(sid, taskID), &v); err == nil {
		c.VerifyPassed = v.Passed()
	}

	if fi, err := os.Stat(l.Handoff(sid, taskID)); err == nil && fi.Size() > 0 {
		c.HandoffWritten = true
	}

	c.OpenGates = countOpenGates(l, sid, taskID)
	return c, nil
}

func countOpenGates(l store.Layout, sid, taskID string) int {
	entries, err := os.ReadDir(l.GatesDir(sid))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var g model.Gate
		if err := store.ReadJSON(filepath.Join(l.GatesDir(sid), e.Name()), &g); err == nil {
			if g.TaskID == taskID && g.Status == "open" {
				n++
			}
		}
	}
	return n
}

func isTerminal(status string) bool {
	switch status {
	case model.JobCompleted, model.JobFailed, model.JobNeedsHuman, model.JobTimeout:
		return true
	}
	return false
}
