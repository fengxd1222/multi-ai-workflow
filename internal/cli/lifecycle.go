package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fengxudong/harness/internal/event"
	"github.com/fengxudong/harness/internal/integrate"
	"github.com/fengxudong/harness/internal/model"
	"github.com/fengxudong/harness/internal/scope"
	"github.com/fengxudong/harness/internal/state"
	"github.com/fengxudong/harness/internal/store"
	"github.com/fengxudong/harness/internal/verify"
)

// VerifyTask runs task-level verification commands itself and writes
// verification.json (rev3 §13). Returns verify-failed when any required check
// fails.
func VerifyTask(dir, sid, taskID, workdir string, cmds []string) error {
	root, l, sid, err := sessionLayout(dir, sid)
	if err != nil {
		return err
	}
	if workdir == "" {
		workdir = root
	}
	v, passed := verify.Run(workdir, "sh", "task", cmds)
	if err := store.WriteAtomicJSON(l.Verification(sid, taskID), v); err != nil {
		return err
	}
	_ = state.New(l, sid).AppendInfo("orchestrator", model.EvVerifyCompleted, map[string]any{
		"task_id": taskID, "passed": passed, "checks": len(v.Checks),
	})
	fmt.Printf("task %s verify: %d check(s), passed=%v\n", taskID, len(v.Checks), passed)
	if !passed {
		return coded(ExitVerifyFailed, "task %s verification failed", taskID)
	}
	return nil
}

// Integrate merges every completed write-job branch for a task into
// harness/integration/<tid> (rev3 §7 §11). Denied changes or merge conflicts
// open a gate and stop.
func Integrate(dir, sid, taskID string) error {
	root, l, sid, err := sessionLayout(dir, sid)
	if err != nil {
		return err
	}
	var branches []integrate.JobBranch
	for _, j := range jobsForTask(l, sid, taskID) {
		if j.Writes && j.Status == model.JobCompleted && j.Branch != nil {
			branches = append(branches, integrate.JobBranch{JobID: j.JobID, Branch: *j.Branch, Scope: j.Scope})
		}
	}
	rsv, _ := scope.LoadReserved(l.Reserved())
	res, err := integrate.IntegrateTask(root, taskID, branches, rsv, ignorecase(root))
	if err != nil {
		return coded(ExitStateCorrupt, "integrate: %v", err)
	}

	eng := state.New(l, sid)
	if res.DeniedJob != "" {
		_ = openGate(eng, l, sid, taskID, res.DeniedJob, "denied-change-at-integrate")
		return coded(ExitNeedsHuman, "integrate blocked: job %s introduced out-of-scope changes", res.DeniedJob)
	}
	if res.ConflictJob != "" {
		_ = openGate(eng, l, sid, taskID, res.ConflictJob, "merge-conflict")
		return coded(ExitNeedsHuman, "integrate blocked: job %s conflicted (merge aborted)", res.ConflictJob)
	}
	_ = eng.AppendInfo("orchestrator", model.EvIntegrateDone, map[string]any{
		"task_id": taskID, "integration_branch": res.IntegrationBranch, "merged": res.Merged,
	})
	fmt.Printf("integrated task %s: %d job(s) -> %s\n", taskID, len(res.Merged), res.IntegrationBranch)
	return nil
}

// Handoff renders the task closeout markdown and records handoff.written
// (rev3 §11). harness delivers the integration branch; this is the human's
// review entry point.
func Handoff(dir, sid, taskID string) error {
	_, l, sid, err := sessionLayout(dir, sid)
	if err != nil {
		return err
	}
	jobs := jobsForTask(l, sid, taskID)
	branch := integrationBranch(l, sid, taskID)

	var b strings.Builder
	fmt.Fprintf(&b, "# Handoff — task %s\n\n", taskID)
	fmt.Fprintf(&b, "## Jobs\n\n| job | role | runtime | status |\n|---|---|---|---|\n")
	for _, j := range jobs {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", j.JobID, j.Role, j.TargetRuntime, j.Status)
	}
	var v model.Verification
	if store.ReadJSON(l.Verification(sid, taskID), &v) == nil {
		fmt.Fprintf(&b, "\n## Verification\n\npassed=%v (%d checks)\n", v.Passed(), len(v.Checks))
	}
	fmt.Fprintf(&b, "\n## Delivery\n\n")
	if branch != "" {
		fmt.Fprintf(&b, "Integrated on branch `%s` — review and merge into your branch.\n", branch)
	} else {
		fmt.Fprintf(&b, "No integration branch (no completed write jobs).\n")
	}

	if err := store.WriteAtomic(l.Handoff(sid, taskID), []byte(b.String()), 0o644); err != nil {
		return err
	}
	_ = state.New(l, sid).AppendInfo("orchestrator", model.EvHandoffWritten, map[string]string{"task_id": taskID})
	fmt.Printf("handoff written for task %s\n", taskID)
	return nil
}

func openGate(eng *state.Engine, l store.Layout, sid, taskID, jobID, reason string) error {
	g := model.Gate{
		GateID: newGateID(), TaskID: taskID, JobID: jobID, Reason: reason,
		Options:     []string{"approve_extra_files", "reject_and_rollback", "reassign_scope"},
		Recommended: "reject_and_rollback", Status: "open", CreatedAt: event.Now(),
	}
	return eng.OpenGate("orchestrator", g)
}

// integrationBranch scans events for the most recent integrate.completed branch.
func integrationBranch(l store.Layout, sid, taskID string) string {
	evs, err := event.Fold(l, sid)
	if err != nil {
		return ""
	}
	branch := ""
	for _, ev := range evs {
		if ev.Type != model.EvIntegrateDone {
			continue
		}
		var p struct {
			TaskID            string `json:"task_id"`
			IntegrationBranch string `json:"integration_branch"`
		}
		if json.Unmarshal(ev.Payload, &p) == nil && p.TaskID == taskID {
			branch = p.IntegrationBranch
		}
	}
	return branch
}

func sessionLayout(dir, sid string) (root string, l store.Layout, outSID string, err error) {
	root, err = repoRoot(dir)
	if err != nil {
		return "", store.Layout{}, "", coded(ExitUsage, "%s is not inside a git repository", dir)
	}
	l = store.NewLayout(root)
	if sid == "" {
		if sid, err = latestSession(l); err != nil {
			return root, l, "", err
		}
	}
	return root, l, sid, nil
}
