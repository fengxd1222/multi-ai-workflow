package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fengxd1222/multi-ai-workflow/internal/model"
	"github.com/fengxd1222/multi-ai-workflow/internal/state"
	"github.com/fengxd1222/multi-ai-workflow/internal/store"
	"github.com/fengxd1222/multi-ai-workflow/internal/worktree"
)

const maxRecoverRetries = 2

// Recover rebuilds views from the event log, then reconciles the working tree
// (rev3 §15): stale running jobs (dead worker) are discarded and reset for
// re-dispatch or escalated to needs-human past the retry cap; orphan worktrees
// (git-registered but with no live job) are pruned.
func Recover(dir, sid string) error {
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

	// recover.lock makes concurrent recovers mutually exclusive (rev3 N6).
	lk, ok, err := store.TryLock(l.RecoverLock())
	if err != nil {
		return err
	}
	if !ok {
		return coded(ExitLockTimeout, "another `harness recover` is in progress")
	}
	defer lk.Release()

	eng := state.New(l, sid)
	jobs, tasks, err := eng.RebuildViews()
	if err != nil {
		return coded(ExitStateCorrupt, "rebuild views for %s: %v", sid, err)
	}

	boot := bootID()
	recovered, escalated := 0, 0
	aliveRunning := map[string]bool{}

	for _, j := range allJobs(l, sid) {
		if j.Status != model.JobRunning {
			continue
		}
		if jobAlive(j, boot) {
			aliveRunning[j.JobID] = true
			continue
		}
		// stale: discard the worktree (write jobs) then reset or escalate (N5/N3)
		if j.Writes {
			_ = worktree.Remove(root, j.JobID)
		}
		nj, err := eng.RecoverJob("recover", j.JobID, maxRecoverRetries)
		if err != nil {
			continue
		}
		if nj.Status == model.JobNeedsHuman {
			escalated++
			_, _ = eng.OpenGateForJob("recover", nj.TaskID, nj.JobID, "recover-retry-exhausted", nil)
		} else {
			recovered++
		}
	}

	orphans := pruneOrphanWorktrees(root, aliveRunning)

	_ = appendSessionEvent(l, sid, "cli", model.EvRecoverRan, map[string]int{
		"jobs": jobs, "tasks": tasks, "recovered": recovered, "escalated": escalated, "orphans": orphans,
	})
	fmt.Printf("recovered session %s: views(j=%d t=%d) stale-reset=%d escalated=%d orphan-worktrees=%d\n",
		sid, jobs, tasks, recovered, escalated, orphans)
	return nil
}

// jobAlive reports whether a running job's worker is still live. No worker
// record means it cannot be verified and is treated as stale (rev3 N5).
func jobAlive(j model.Job, currentBoot string) bool {
	if j.Worker == nil {
		return false
	}
	return processAlive(j.Worker.PID, j.Worker.BootID, currentBoot)
}

// pruneOrphanWorktrees removes git-registered worktrees under .worktrees/ that
// no live running job owns, plus their residual branches (rev3 N9). Integration
// worktrees (__int__*) are always transient and removed.
func pruneOrphanWorktrees(root string, aliveRunning map[string]bool) int {
	out, err := git(root, "worktree", "list", "--porcelain")
	if err != nil {
		return 0
	}
	base := filepath.Join(root, ".worktrees")
	n := 0
	for _, line := range strings.Split(out, "\n") {
		path, ok := strings.CutPrefix(line, "worktree ")
		if !ok {
			continue
		}
		if filepath.Dir(path) != base {
			continue // main worktree or unrelated
		}
		name := filepath.Base(path)
		isInt := strings.HasPrefix(name, "__int__")
		if !isInt && aliveRunning[name] {
			continue // owned by a live job
		}
		_, _ = git(root, "worktree", "remove", "--force", path)
		_, _ = git(root, "worktree", "prune")
		if !isInt {
			_, _ = git(root, "branch", "-D", "job/"+name)
		}
		n++
	}
	return n
}

// allJobs reads every job view in the session.
func allJobs(l store.Layout, sid string) []model.Job {
	entries, err := os.ReadDir(l.JobsDir(sid))
	if err != nil {
		return nil
	}
	var jobs []model.Job
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var j model.Job
		if store.ReadJSON(filepath.Join(l.JobsDir(sid), e.Name()), &j) == nil {
			jobs = append(jobs, j)
		}
	}
	return jobs
}

// latestSession returns the lexicographically-greatest session id (S-<date>-…
// sorts chronologically).
func latestSession(l store.Layout) (string, error) {
	entries, err := os.ReadDir(l.Sessions())
	if err != nil {
		return "", coded(ExitUsage, "no sessions found; run `harness session start`")
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "S-") {
			ids = append(ids, e.Name())
		}
	}
	if len(ids) == 0 {
		return "", coded(ExitUsage, "no sessions found; run `harness session start`")
	}
	sort.Strings(ids)
	return ids[len(ids)-1], nil
}
