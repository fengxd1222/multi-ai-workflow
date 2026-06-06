package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/fengxudong/harness/internal/adapter"
	"github.com/fengxudong/harness/internal/model"
	"github.com/fengxudong/harness/internal/runtime"
	"github.com/fengxudong/harness/internal/scope"
	"github.com/fengxudong/harness/internal/state"
	"github.com/fengxudong/harness/internal/store"
	"github.com/fengxudong/harness/internal/trellis"
)

// Run drives one created job to a terminal state via the adapter (rev3 §8, §16).
// The runtime binary can be overridden with HARNESS_CODEX_BIN / HARNESS_CLAUDE_BIN.
func Run(dir, sid, jobID string) error {
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

	var job model.Job
	if err := store.ReadJSON(l.JobView(sid, jobID), &job); err != nil {
		return coded(ExitUsage, "job %s not found in session %s", jobID, sid)
	}

	rt, err := runtimeFor(job.TargetRuntime)
	if err != nil {
		return err
	}
	rsv, _ := scope.LoadReserved(l.Reserved())

	a := &adapter.Adapter{
		L: l, SID: sid, Eng: state.New(l, sid), RT: rt,
		Reserved: rsv, Ignorecase: ignorecase(root), BootID: bootID(),
	}
	out, err := a.Run(context.Background(), jobID)
	if err != nil {
		return coded(ExitRuntimeFailed, "run job %s: %v", jobID, err)
	}
	fmt.Printf("job %s -> %s (%s)\n", jobID, out.Status, out.Reason)

	recordTrellisJournal(root, out.Job)

	if out.Status == model.JobNeedsHuman {
		return coded(ExitNeedsHuman, "job %s needs human review", jobID)
	}
	return nil
}

// recordTrellisJournal writes a journal entry back to a co-located Trellis
// workspace via its own add_session.py (so the format stays Trellis's). Strictly
// best-effort: no Trellis task, no script, or no python interpreter all skip
// quietly with a note — never fails the run (write-back strategy: scripts).
func recordTrellisJournal(root string, job model.Job) {
	if job.TrellisTask == "" {
		return
	}
	proj, ok := trellis.Detect(root)
	if !ok {
		return
	}
	if !proj.HasScript("add_session.py") {
		fmt.Println("note: .trellis/scripts/add_session.py not found — skipping Trellis journal write-back")
		return
	}
	branch := ""
	if job.Branch != nil {
		branch = *job.Branch
	}
	title := fmt.Sprintf("harness %s — %s", job.Status, job.Goal)
	summary := fmt.Sprintf("harness job %s (%s/%s) on task %s; branch=%s",
		job.JobID, job.TargetRuntime, job.Role, job.TrellisTask, branch)
	if _, err := proj.RecordSession(title, "", summary); err != nil {
		fmt.Printf("note: Trellis journal write-back skipped (%v; set HARNESS_PYTHON if your interpreter is non-standard)\n", err)
		return
	}
	fmt.Printf("recorded session to Trellis journal for task %s\n", job.TrellisTask)
}

func runtimeFor(target string) (runtime.Runtime, error) {
	switch target {
	case model.RuntimeCodex:
		return runtime.Codex{Bin: os.Getenv("HARNESS_CODEX_BIN")}, nil
	case model.RuntimeClaude:
		return runtime.Claude{Bin: os.Getenv("HARNESS_CLAUDE_BIN")}, nil
	default:
		return nil, coded(ExitUsage, "unknown target runtime %q", target)
	}
}

// bootID returns a host-boot identifier used to detect pid reuse across reboots
// (rev3 N1/N5). Best-effort; empty if unavailable.
func bootID() string {
	if b, err := os.ReadFile("/proc/sys/kernel/random/boot_id"); err == nil {
		return strings.TrimSpace(string(b))
	}
	if out, err := exec.Command("sysctl", "-n", "kern.boottime").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}
