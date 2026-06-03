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
	if out.Status == model.JobNeedsHuman {
		return coded(ExitNeedsHuman, "job %s needs human review", jobID)
	}
	return nil
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
