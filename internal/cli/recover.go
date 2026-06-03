package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/fengxudong/harness/internal/model"
	"github.com/fengxudong/harness/internal/state"
	"github.com/fengxudong/harness/internal/store"
)

// Recover rebuilds materialized views from the event log under recover.lock.
// In M1 this is replay-only; worktree/orphan reconciliation is M6 (rev3 §15).
func Recover(dir, sid string) error {
	root, err := repoRoot(dir)
	if err != nil {
		return coded(ExitUsage, "%s is not inside a git repository", dir)
	}
	l := store.NewLayout(root)

	if sid == "" {
		sid, err = latestSession(l)
		if err != nil {
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

	_ = appendSessionEvent(l, sid, "cli", model.EvRecoverRan, map[string]int{
		"jobs": jobs, "tasks": tasks,
	})
	fmt.Printf("recovered session %s: rebuilt %d job view(s), %d task view(s)\n", sid, jobs, tasks)
	return nil
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
