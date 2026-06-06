package cli

import (
	"fmt"
	"io"

	"github.com/fengxd1222/multi-ai-workflow/internal/guard"
	"github.com/fengxd1222/multi-ai-workflow/internal/model"
	"github.com/fengxd1222/multi-ai-workflow/internal/scope"
	"github.com/fengxd1222/multi-ai-workflow/internal/state"
	"github.com/fengxd1222/multi-ai-workflow/internal/store"
)

// GuardPretool is the PreToolUse entry point (rev3 §10 §11): it parses the hook
// payload, evaluates it against the job's scope + reserved + dangerous-command
// policy, writes the runtime's decision JSON to out, and records a violation
// event on non-allow. Fail-safe: a payload it cannot parse is gated, not allowed.
func GuardPretool(dir, repo, sid, jobID, rt string, in io.Reader, out io.Writer) error {
	l, sid, job, err := loadJob(dir, repo, sid, jobID)
	if err != nil {
		return err
	}
	payload, _ := io.ReadAll(in)

	tc, perr := guard.ParsePreTool(rt, payload)
	if perr != nil {
		_, _ = out.Write(guard.FormatDecision(rt, guard.Gate, "unparseable-hook-payload"))
		return nil
	}

	rsv, _ := scope.LoadReserved(l.Reserved())
	res := guard.EvaluatePreTool(tc, job.Workdir, job.Scope, rsv, ignorecase(l.RepoRoot))
	_, _ = out.Write(guard.FormatDecision(rt, res.Decision, res.Reason))

	if res.Decision != guard.Allow {
		_ = state.New(l, sid).AppendInfo("guard:"+jobID, model.EvScopeViolation, map[string]any{
			"job_id": jobID, "tool": tc.Tool, "decision": res.Decision.String(), "reason": res.Reason,
		})
	}
	return nil
}

// GuardPosttool is the PostToolUse / migration-point diff review (rev3 §10 §11):
// it recomputes the real change set in the job's worktree and records any
// out-of-scope writes the worker did NOT report (C3). Returns blocked-policy
// when violations are found so a hook can react.
func GuardPosttool(dir, repo, sid, jobID string) error {
	l, sid, job, err := loadJob(dir, repo, sid, jobID)
	if err != nil {
		return err
	}
	if !job.Writes || job.BaseCommit == nil || *job.BaseCommit == "" {
		return nil // read-only job: nothing to diff
	}

	rsv, _ := scope.LoadReserved(l.Reserved())
	violations, err := guard.PostToolReview(job.Workdir, *job.BaseCommit, job.Scope, rsv, ignorecase(l.RepoRoot))
	if err != nil {
		return coded(ExitStateCorrupt, "post-tool review: %v", err)
	}
	if len(violations) == 0 {
		return nil
	}
	_ = state.New(l, sid).AppendInfo("guard:"+jobID, model.EvScopeViolation, map[string]any{
		"job_id": jobID, "violations": violations,
	})
	fmt.Printf("post-tool review: %d scope violation(s) in %s\n", len(violations), jobID)
	return coded(ExitBlockedPolicy, "scope violations detected")
}

// loadJob resolves a job view. repo, when non-empty, overrides the cwd-derived
// repo root — required because hooks fire inside a worktree but the .harness
// state lives in the main repo (rev3 N4/F4).
func loadJob(dir, repo, sid, jobID string) (store.Layout, string, model.Job, error) {
	root := repo
	var err error
	if root == "" {
		if root, err = repoRoot(dir); err != nil {
			return store.Layout{}, "", model.Job{}, coded(ExitUsage, "%s is not inside a git repository", dir)
		}
	}
	l := store.NewLayout(root)
	if sid == "" {
		if sid, err = latestSession(l); err != nil {
			return l, "", model.Job{}, err
		}
	}
	var job model.Job
	if err := store.ReadJSON(l.JobView(sid, jobID), &job); err != nil {
		return l, sid, job, coded(ExitUsage, "job %s not found in session %s", jobID, sid)
	}
	return l, sid, job, nil
}
