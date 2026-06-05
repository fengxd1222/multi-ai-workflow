package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fengxudong/harness/internal/event"
	"github.com/fengxudong/harness/internal/model"
	"github.com/fengxudong/harness/internal/state"
	"github.com/fengxudong/harness/internal/store"
)

// DelegateSpec describes a job to create from a task (rev3 §8, §16). In the
// CLI+hooks model the orchestrator (a human-driven session) supplies these.
type DelegateSpec struct {
	TaskID      string
	Role        string
	Goal        string
	Brief       string
	Runtime     string
	Allowed     []string
	Denied      []string
	Constraints []string
	Context     []string // repo-relative files the worker must Read first
	From        []string // upstream job ids whose findings to attach as context
	Verify      []string
	Depth       int
	ParentChain []string // ancestor delegation fingerprints (for cycle detection)
}

// Delegate creates a job (status created) the adapter can then run. Mode/writes
// derive from the role (write roles → worktree).
func Delegate(dir, sid string, spec DelegateSpec) (string, error) {
	root, err := repoRoot(dir)
	if err != nil {
		return "", coded(ExitUsage, "%s is not inside a git repository", dir)
	}
	l := store.NewLayout(root)
	if sid == "" {
		if sid, err = latestSession(l); err != nil {
			return "", err
		}
	}
	if spec.Role == "" || spec.Runtime == "" || spec.TaskID == "" {
		return "", coded(ExitUsage, "delegate needs --task, --role and --runtime")
	}

	// Delegation depth + cycle detection (rev3 §8.5 N29).
	fp := spec.TaskID + ":" + spec.Role
	chain := []string{fp}
	depth := spec.Depth
	if depth == 0 {
		depth = 1
	}
	if len(spec.ParentChain) > 0 {
		for _, f := range spec.ParentChain {
			if f == fp {
				return "", coded(ExitDelegationLoop, "delegation cycle: %s already in ancestor chain", fp)
			}
		}
		chain = append(append([]string{}, spec.ParentChain...), fp)
		depth = len(chain)
	}
	if depth > maxDelegationDepth {
		return "", coded(ExitDelegationLoop, "delegation depth %d exceeds max %d", depth, maxDelegationDepth)
	}

	// Task token budget is a soft gate: it only blocks the NEXT dispatch (rev3 §8.4 N28).
	if budget := taskBudget(l, sid, spec.TaskID); budget > 0 {
		if used := sumTaskUsage(l, sid, spec.TaskID); used >= budget {
			return "", coded(ExitBudgetExceeded, "task %s used %d >= budget %d tokens", spec.TaskID, used, budget)
		}
	}

	writes := model.RoleWrites(spec.Role)
	mode := model.ModeShared
	if writes {
		mode = model.ModeWorktree
	}

	// Grounding: explicit --context files plus the findings of any --from upstream
	// jobs (typically a prior analysis/research job). Worker Reads these first.
	ctxRefs := append([]string{}, spec.Context...)
	for _, up := range spec.From {
		fp := l.Findings(sid, up)
		if _, err := os.Stat(fp); err != nil {
			return "", coded(ExitUsage, "--from %s: no findings (is it a completed read-only job?)", up)
		}
		if rel, err := filepath.Rel(root, fp); err == nil {
			ctxRefs = append(ctxRefs, rel)
		}
	}

	jobID := newJobID()
	job := model.Job{
		JobID: jobID, TaskID: spec.TaskID, CreatedBy: "orchestrator", TargetRuntime: spec.Runtime,
		Role: spec.Role, Goal: spec.Goal, Brief: spec.Brief, Writes: writes, Mode: mode,
		StateRoot: l.StateRoot, RepoRoot: root, Workdir: root,
		Scope:                    model.Scope{Allowed: spec.Allowed, Denied: spec.Denied},
		Constraints:              spec.Constraints,
		ContextRefs:              ctxRefs,
		VerificationRequirements: spec.Verify,
		Delegation:               model.Delegation{Depth: depth, ChainFingerprints: chain},
		Budget:                   model.JobBudget{MaxTokens: 200000, TimeoutS: 1800},
		ResultContract:           "job-result.schema.json",
	}
	if err := state.New(l, sid).CreateJob("orchestrator", job); err != nil {
		return "", err
	}
	fmt.Printf("delegated %s (%s/%s) for task %s\n", jobID, spec.Runtime, spec.Role, spec.TaskID)
	return jobID, nil
}

const maxDelegationDepth = 3

// taskBudget returns the task's token budget, or 0 if none.
func taskBudget(l store.Layout, sid, taskID string) int {
	var t model.Task
	if store.ReadJSON(l.TaskView(sid, taskID), &t) == nil && t.Budget != nil {
		return t.Budget.MaxTokens
	}
	return 0
}

// sumTaskUsage folds usage.reported events for the task's jobs (rev3 §8.4 N28:
// usage is summed from append-only events, never a shared mutable counter).
func sumTaskUsage(l store.Layout, sid, taskID string) int {
	owned := map[string]bool{}
	for _, j := range jobsForTask(l, sid, taskID) {
		owned[j.JobID] = true
	}
	evs, err := event.Fold(l, sid)
	if err != nil {
		return 0
	}
	total := 0
	for _, ev := range evs {
		if ev.Type != model.EvUsageReported {
			continue
		}
		var p struct {
			JobID  string `json:"job_id"`
			Tokens int    `json:"tokens"`
		}
		if json.Unmarshal(ev.Payload, &p) == nil && owned[p.JobID] {
			total += p.Tokens
		}
	}
	return total
}
