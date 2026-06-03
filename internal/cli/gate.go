package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fengxudong/harness/internal/event"
	"github.com/fengxudong/harness/internal/model"
	"github.com/fengxudong/harness/internal/state"
	"github.com/fengxudong/harness/internal/store"
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

// GateApprove resolves a gate as approved. For approve_extra_files it persists
// the approved paths into the job's scope so re-review won't re-gate them
// (rev3 §14 N30).
func GateApprove(dir, sid, gateID, option string, files []string) error {
	_, l, sid, err := sessionLayout(dir, sid)
	if err != nil {
		return err
	}
	eng := state.New(l, sid)
	var g model.Gate
	if err := store.ReadJSON(l.GateView(sid, gateID), &g); err != nil {
		return coded(ExitUsage, "gate %s not found", gateID)
	}

	if option == "approve_extra_files" && len(files) > 0 {
		var job model.Job
		if err := store.ReadJSON(l.JobView(sid, g.JobID), &job); err != nil {
			return coded(ExitUsage, "job %s not found for gate", g.JobID)
		}
		if _, err := eng.ExtendJobScope("human", g.JobID, job.Rev, files); err != nil {
			return err
		}
	}

	res := &model.Resolution{Option: option, ResolvedAt: event.Now(), By: "human"}
	if _, err := eng.ResolveGate("human", gateID, "approved", res); err != nil {
		return err
	}
	fmt.Printf("gate %s approved (%s)\n", gateID, option)
	return nil
}

// GateReject resolves a gate as rejected.
func GateReject(dir, sid, gateID string) error {
	_, l, sid, err := sessionLayout(dir, sid)
	if err != nil {
		return err
	}
	res := &model.Resolution{Option: "reject_and_rollback", ResolvedAt: event.Now(), By: "human"}
	if _, err := state.New(l, sid).ResolveGate("human", gateID, "rejected", res); err != nil {
		return coded(ExitUsage, "gate %s not found", gateID)
	}
	fmt.Printf("gate %s rejected\n", gateID)
	return nil
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
