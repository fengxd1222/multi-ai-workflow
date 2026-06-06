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

	writeBackTrellis(root, out.Job)

	if out.Status == model.JobNeedsHuman {
		return coded(ExitNeedsHuman, "job %s needs human review", jobID)
	}
	return nil
}

// writeBackTrellis writes harness's outcome back to a co-located Trellis task via
// Trellis's own scripts (so formats stay Trellis's): records the job's branch on
// the task (set-branch, only touches task.json) and appends a journal entry
// (add_session.py --no-commit). Strictly best-effort — missing task/script/python
// all skip with a note, never failing the run. status推进(start/finish/archive)
// 不自动: start 需 Trellis session identity, archive 涉 review 边界, 由你在 Trellis 侧驱动.
func writeBackTrellis(root string, job model.Job) {
	if job.TrellisTask == "" {
		return
	}
	proj, ok := trellis.Detect(root)
	if !ok {
		return
	}
	branch := ""
	if job.Branch != nil {
		branch = *job.Branch
	}

	// set-branch: record the job's git branch on the Trellis task (safe — only
	// rewrites task.json.branch, no git commit).
	if branch != "" && proj.HasScript("task.py") {
		if _, err := proj.SetBranch(job.TrellisTask, branch); err != nil {
			fmt.Printf("note: trellis set-branch skipped (%v)\n", err)
		}
	}

	// journal
	if !proj.HasScript("add_session.py") {
		fmt.Println("note: .trellis/scripts/add_session.py not found — skipping Trellis journal")
		return
	}
	title := fmt.Sprintf("harness %s — %s", job.Status, job.Goal)
	summary := fmt.Sprintf("harness job %s (%s/%s) on task %s", job.JobID, job.TargetRuntime, job.Role, job.TrellisTask)
	if _, err := proj.RecordSession(title, "", summary, branch); err != nil {
		fmt.Printf("note: Trellis journal skipped (%v; set HARNESS_PYTHON if your interpreter is non-standard)\n", err)
		return
	}
	fmt.Printf("trellis: journaled task %s\n", job.TrellisTask)
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
