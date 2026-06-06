package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fengxd1222/multi-ai-workflow/internal/event"
	"github.com/fengxd1222/multi-ai-workflow/internal/model"
	"github.com/fengxd1222/multi-ai-workflow/internal/state"
	"github.com/fengxd1222/multi-ai-workflow/internal/store"
	"github.com/fengxd1222/multi-ai-workflow/internal/worktree"
)

// GateList prints open and resolved gates (rev3 §14).
func GateList(dir, sid string) error {
	_, l, sid, err := sessionLayout(dir, sid)
	if err != nil {
		return err
	}
	for _, g := range loadGates(l, sid) {
		fmt.Printf("%s  [%s]  task=%s job=%s  %s\n", g.GateID, g.Status, g.TaskID, g.JobID, g.Reason)
	}
	return nil
}

// GateShow prints one gate.
func GateShow(dir, sid, gateID string) error {
	_, l, sid, err := sessionLayout(dir, sid)
	if err != nil {
		return err
	}
	var g model.Gate
	if err := store.ReadJSON(l.GateView(sid, gateID), &g); err != nil {
		return coded(ExitUsage, "gate %s not found", gateID)
	}
	fmt.Printf("gate %s\n  status: %s\n  task: %s\n  job: %s\n  reason: %s\n  options: %s\n  affected: %s\n",
		g.GateID, g.Status, g.TaskID, g.JobID, g.Reason,
		strings.Join(g.Options, ","), strings.Join(g.AffectedFiles, ","))
	return nil
}

// GateApprove resolves a gate as approved: for approve_extra_files it persists
// the paths into the job's scope (N30), then moves the job out of needs-human so
// task completion isn't blocked — a non-completed job is reset to created for a
// clean re-run and its worktree discarded (review findings 1 & 2).
func GateApprove(dir, sid, gateID, option string, files []string) error {
	root, l, sid, err := sessionLayout(dir, sid)
	if err != nil {
		return err
	}
	eng := state.New(l, sid)
	var g model.Gate
	if err := store.ReadJSON(l.GateView(sid, gateID), &g); err != nil {
		return coded(ExitUsage, "gate %s not found", gateID)
	}

	var job model.Job
	hasJob := store.ReadJSON(l.JobView(sid, g.JobID), &job) == nil

	if option == "approve_extra_files" && len(files) > 0 && hasJob {
		if _, err := eng.ExtendJobScope("human", g.JobID, job.Rev, files); err != nil {
			return err
		}
		_ = store.ReadJSON(l.JobView(sid, g.JobID), &job) // refresh rev
	}

	res := &model.Resolution{Option: option, ResolvedAt: event.Now(), By: "human"}
	if _, err := eng.ResolveGate("human", gateID, "approved", res); err != nil {
		return err
	}

	next := fmt.Sprintf("`harness integrate --task %s` to retry with the approved scope", g.TaskID)
	if hasJob && nonCompletedTerminal(job.Status) {
		if _, err := eng.ResolveJob("human", g.JobID, model.JobCreated); err == nil {
			_ = worktree.Remove(root, g.JobID)
			next = fmt.Sprintf("`harness run --job %s` to re-run with the approved scope, then `harness integrate --task %s`", g.JobID, g.TaskID)
		}
	}
	fmt.Printf("gate %s approved (%s)\nnext: %s\n", gateID, option, next)
	return nil
}

// GateReject resolves a gate as rejected: the job's work is abandoned (cancelled,
// excluded from completion) and its worktree discarded so the task can be closed
// out via a corrected re-delegation (review findings 1 & 2).
func GateReject(dir, sid, gateID string) error {
	root, l, sid, err := sessionLayout(dir, sid)
	if err != nil {
		return err
	}
	eng := state.New(l, sid)
	var g model.Gate
	if err := store.ReadJSON(l.GateView(sid, gateID), &g); err != nil {
		return coded(ExitUsage, "gate %s not found", gateID)
	}
	res := &model.Resolution{Option: "reject_and_rollback", ResolvedAt: event.Now(), By: "human"}
	if _, err := eng.ResolveGate("human", gateID, "rejected", res); err != nil {
		return err
	}
	if _, err := eng.ResolveJob("human", g.JobID, model.JobCancelled); err == nil {
		_ = worktree.Remove(root, g.JobID)
	}
	fmt.Printf("gate %s rejected\nnext: job %s cancelled; re-delegate a corrected job if needed, then `harness integrate --task %s`\n",
		gateID, g.JobID, g.TaskID)
	return nil
}

func nonCompletedTerminal(status string) bool {
	switch status {
	case model.JobNeedsHuman, model.JobFailed, model.JobTimeout:
		return true
	}
	return false
}

func loadGates(l store.Layout, sid string) []model.Gate {
	entries, err := os.ReadDir(l.GatesDir(sid))
	if err != nil {
		return nil
	}
	var gates []model.Gate
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var g model.Gate
		if store.ReadJSON(filepath.Join(l.GatesDir(sid), e.Name()), &g) == nil {
			gates = append(gates, g)
		}
	}
	return gates
}
