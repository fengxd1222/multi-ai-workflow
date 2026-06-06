package cli

import (
	"fmt"
	"os"

	"github.com/fengxd1222/multi-ai-workflow/internal/model"
	"github.com/fengxd1222/multi-ai-workflow/internal/worktree"
)

// prunable job statuses: terminal jobs whose worktree checkout is no longer
// needed. needs-human is excluded — its worktree is the gate's evidence;
// running/created are obviously kept.
var prunable = map[string]bool{
	model.JobCompleted: true,
	model.JobFailed:    true,
	model.JobTimeout:   true,
	model.JobCancelled: true,
}

// Prune reclaims the worktree checkouts of terminal write jobs, keeping their
// job/<id> branches. Manual GC so worktrees don't accumulate across many jobs
// (integrate already reclaims merged ones; this catches the rest).
func Prune(dir, sid string) error {
	root, l, sid, err := sessionLayout(dir, sid)
	if err != nil {
		return err
	}
	n := 0
	for _, j := range allJobs(l, sid) {
		if !j.Writes || !prunable[j.Status] {
			continue
		}
		if _, err := os.Stat(worktree.Path(root, j.JobID)); err == nil {
			_ = worktree.RemoveWorktreeOnly(root, j.JobID)
			n++
		}
	}
	fmt.Printf("pruned %d worktree(s) (terminal write jobs; branches kept)\n", n)
	return nil
}
