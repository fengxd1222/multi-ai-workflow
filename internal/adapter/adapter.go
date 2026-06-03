// Package adapter is the push-model orchestration glue (rev3 §8): for one job it
// sets up the worktree, runs the worker via an injected Runtime under a
// watchdog, captures artifacts, extracts and schema-validates the result (with
// one repair retry), then establishes ground truth — changed files via git and
// verification by re-running the commands itself — before CAS-transitioning the
// job to its terminal state. Worker self-reports are never trusted (C3).
package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fengxudong/harness/internal/event"
	"github.com/fengxudong/harness/internal/model"
	"github.com/fengxudong/harness/internal/runtime"
	"github.com/fengxudong/harness/internal/scope"
	"github.com/fengxudong/harness/internal/state"
	"github.com/fengxudong/harness/internal/store"
	"github.com/fengxudong/harness/internal/worktree"
)

// Adapter drives one runtime for one session.
type Adapter struct {
	L          store.Layout
	SID        string
	Eng        *state.Engine
	RT         runtime.Runtime
	Reserved   scope.Reserved
	Ignorecase bool
	BootID     string
	MaxRepair  int    // schema-repair retries; default 1
	Shell      string // verify shell; default "sh"
}

// Outcome reports how a job ended and why.
type Outcome struct {
	Job        model.Job
	Status     string
	Reason     string
	Violations []scope.Verdict
}

// Run executes a created job to a terminal state (rev3 §8).
func (a *Adapter) Run(ctx context.Context, jobID string) (Outcome, error) {
	var job model.Job
	if err := store.ReadJSON(a.L.JobView(a.SID, jobID), &job); err != nil {
		return Outcome{}, fmt.Errorf("load job %s: %w", jobID, err)
	}
	if job.Status != model.JobCreated {
		return Outcome{}, fmt.Errorf("job %s is %s, expected created", jobID, job.Status)
	}
	actor := "worker:" + jobID

	// 1-2. worktree binding (write jobs only); read-only jobs run in main tree.
	workdir, branch, base := job.RepoRoot, "", ""
	if job.Writes {
		wt, err := worktree.Add(job.RepoRoot, jobID)
		if err != nil {
			return Outcome{}, err
		}
		workdir, branch, base = wt.Workdir, wt.Branch, wt.BaseCommit
		_ = a.Eng.AppendInfo(actor, model.EvWorktreeCreated, map[string]string{
			"job_id": jobID, "path": workdir, "branch": branch, "base_commit": base,
		})
	}

	// 4. created -> running, stamping worker identity + worktree binding.
	worker := &model.Worker{PID: os.Getpid(), BootID: a.BootID, RunningSince: event.Now()}
	running, err := a.Eng.TransitionJobRunning(actor, jobID, job.Rev, worker, workdir, branch, base)
	if err != nil {
		return Outcome{}, err
	}

	// 3+5. build prompt, run under watchdog, capture artifacts.
	res, perr := a.invoke(ctx, running, workdir)
	if perr != nil {
		return Outcome{}, perr
	}
	if err := a.writeArtifacts(jobID, res); err != nil {
		return Outcome{}, err
	}

	// Commit the worker's changes onto the job branch so the result is captured
	// deterministically by the CLI (not left to the worker) and is mergeable at
	// integrate (rev3 §7). Done before scope review so the diff sees it. A commit
	// failure means we cannot capture the worker's output on the branch, so the
	// job must NOT be marked completed (review finding 2).
	if running.Writes && res.ExitCode == 0 && res.FinalJSONOK {
		if _, cerr := commitWorktree(workdir, jobID); cerr != nil {
			_ = os.WriteFile(filepath.Join(a.L.Artifacts(a.SID, jobID), "commit-error.txt"), []byte(cerr.Error()), 0o644)
			return a.finish(actor, running, model.JobNeedsHuman, "worktree-commit-failed", nil)
		}
	}

	// 6-7. decide terminal state from process layer + ground truth.
	status, reason, violations := a.decide(actor, running, workdir, base, res)
	return a.finish(actor, running, status, reason, violations)
}

// finish performs the terminal CAS transition and, whenever a job enters
// needs-human, opens a gate so the human has something actionable (review
// finding 1).
func (a *Adapter) finish(actor string, job model.Job, status, reason string, violations []scope.Verdict) (Outcome, error) {
	final, err := a.Eng.TransitionJob(actor, job.JobID, job.Rev, status)
	if err != nil {
		return Outcome{}, err
	}
	if status == model.JobNeedsHuman {
		_, _ = a.Eng.OpenGateForJob(actor, job.TaskID, job.JobID, reason, verdictPaths(violations))
	}
	return Outcome{Job: final, Status: status, Reason: reason, Violations: violations}, nil
}

func verdictPaths(vs []scope.Verdict) []string {
	if len(vs) == 0 {
		return nil
	}
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Path)
	}
	return out
}

// invoke builds the request, runs under a timeout watchdog, and (for the
// schema-repair path) retries once with the prior error fed back (rev3 §8.3).
func (a *Adapter) invoke(ctx context.Context, job model.Job, workdir string) (runtime.Result, error) {
	timeout := job.Budget.TimeoutS
	if timeout <= 0 {
		timeout = 1800
	}
	artifacts := a.L.Artifacts(a.SID, job.JobID)
	if err := os.MkdirAll(artifacts, 0o755); err != nil {
		return runtime.Result{}, err
	}
	req := runtime.Request{
		JobID:         job.JobID,
		Runtime:       job.TargetRuntime,
		Workdir:       workdir,
		Prompt:        a.buildPrompt(job, workdir),
		SchemaPath:    filepath.Join(a.L.Schemas(), "job-result.schema.json"),
		FinalJSONPath: filepath.Join(artifacts, "final.json"),
		TimeoutS:      timeout,
	}
	// sandbox-first scope (rev3 §10): give write jobs write tools, read-only jobs none.
	if job.Writes {
		req.AllowedTools = []string{"Read", "Edit", "Write", "Bash"}
		req.Sandbox = "workspace-write"
	} else {
		req.AllowedTools = []string{"Read", "Grep", "Glob"}
		req.Sandbox = "read-only"
	}

	wctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	res, err := a.RT.Run(wctx, req)
	return res, err
}

// decide maps process result + ground truth to a terminal job status.
func (a *Adapter) decide(actor string, job model.Job, workdir, base string, res runtime.Result) (status, reason string, violations []scope.Verdict) {
	switch {
	case res.KilledByWatchdog:
		return model.JobTimeout, "watchdog timeout", nil
	case res.ExitCode != 0:
		return model.JobFailed, fmt.Sprintf("runtime-exec-failed exit=%d", res.ExitCode), nil
	case !res.FinalJSONOK:
		return model.JobFailed, "runtime-exec-failed torn final.json", nil
	}

	// schema validate with one repair retry (rev3 §8.3)
	r, verr := model.ParseJobResult(res.FinalJSON)
	if verr != nil {
		repaired := a.repair(actor, job, workdir, verr)
		if repaired == nil {
			return model.JobFailed, "result-invalid after repair", nil
		}
		r = *repaired
	}

	// ground truth: changed files via git (NOT worker self-report) — write jobs only
	if job.Writes && base != "" {
		v, err := a.scopeReview(job, workdir, base)
		if err == nil && len(v) > 0 {
			_ = a.Eng.AppendInfo(actor, model.EvScopeViolation, map[string]any{
				"job_id": job.JobID, "violations": v,
			})
			return model.JobNeedsHuman, "scope violation", v
		}
	}

	// ground truth: verification re-run by the CLI (NOT worker self-report)
	if ok, checks := a.runVerify(workdir, job.VerificationRequirements); !ok {
		_ = a.Eng.AppendInfo(actor, model.EvVerifyCompleted, map[string]any{
			"job_id": job.JobID, "passed": false, "checks": checks,
		})
		return model.JobFailed, "verify-failed", nil
	}

	a.recordUsage(actor, job.JobID, res.ReportedTokens, r)
	return model.JobCompleted, "ok", nil
}

func (a *Adapter) repair(actor string, job model.Job, workdir string, prior error) *model.JobResult {
	max := a.MaxRepair
	if max <= 0 {
		max = 1
	}
	for i := 0; i < max; i++ {
		req := runtime.Request{
			JobID: job.JobID, Runtime: job.TargetRuntime, Workdir: workdir,
			Prompt:        a.buildPrompt(job, workdir),
			SchemaPath:    filepath.Join(a.L.Schemas(), "job-result.schema.json"),
			FinalJSONPath: filepath.Join(a.L.Artifacts(a.SID, job.JobID), "final.json"),
			TimeoutS:      job.Budget.TimeoutS,
			RepairOf:      prior.Error(),
		}
		res, err := a.RT.Run(context.Background(), req)
		if err != nil || !res.FinalJSONOK {
			continue
		}
		if r, e := model.ParseJobResult(res.FinalJSON); e == nil {
			return &r
		}
	}
	return nil
}

func (a *Adapter) scopeReview(job model.Job, workdir, base string) ([]scope.Verdict, error) {
	changed, err := scope.ChangedSet(workdir, base)
	if err != nil {
		return nil, err
	}
	var v []scope.Verdict
	for path := range changed {
		verdict := scope.Classify(path, job.Scope, a.Reserved, a.Ignorecase)
		if verdict.Decision != scope.Allow {
			v = append(v, verdict)
		}
	}
	return v, nil
}

type checkResult struct {
	Command  string `json:"command"`
	Passed   bool   `json:"passed"`
	ExitCode int    `json:"exit_code"`
}

func (a *Adapter) runVerify(workdir string, cmds []string) (bool, []checkResult) {
	sh := a.Shell
	if sh == "" {
		sh = "sh"
	}
	allOK := true
	var checks []checkResult
	for _, c := range cmds {
		cmd := exec.Command(sh, "-c", c)
		cmd.Dir = workdir
		err := cmd.Run()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			code = -1
		}
		passed := err == nil
		if !passed {
			allOK = false
		}
		checks = append(checks, checkResult{Command: c, Passed: passed, ExitCode: code})
	}
	return allOK, checks
}

func (a *Adapter) recordUsage(actor, jobID string, reported *int, r model.JobResult) {
	tokens := 0
	estimated := false
	switch {
	case reported != nil:
		tokens = *reported
	case r.Usage != nil:
		tokens = r.Usage.Tokens // informational fallback
		estimated = true
	default:
		estimated = true // conservative: unaccountable usage (rev3 N28)
	}
	_ = a.Eng.AppendInfo(actor, model.EvUsageReported, map[string]any{
		"job_id": jobID, "tokens": tokens, "estimated": estimated,
	})
}

func (a *Adapter) buildPrompt(job model.Job, workdir string) string {
	packet := map[string]any{
		"task_id": job.TaskID, "job_id": job.JobID, "role": job.Role,
		"workdir": workdir, "scope": job.Scope,
		"verification_requirements": job.VerificationRequirements,
		"delegation":                job.Delegation,
	}
	b, _ := json.Marshal(packet)
	if len(b) > 4096 { // rev3 §3.5 N38 — externalize instead of inlining
		b = []byte(`{"error":"context-packet exceeds 4KiB; externalize large fields"}`)
	}

	var sb strings.Builder
	sb.WriteString("You are a harness worker executing one job.\n")
	if job.Goal != "" {
		fmt.Fprintf(&sb, "Goal: %s\n", job.Goal)
	}
	if job.Writes {
		fmt.Fprintf(&sb, "Make the necessary file changes with your tools. You may ONLY write within these path globs: %s. Do not write anything outside them.\n",
			strings.Join(job.Scope.Allowed, ", "))
	} else {
		sb.WriteString("This is a read-only job; do not modify files.\n")
	}
	fmt.Fprintf(&sb, "When finished, output ONLY this JSON object and nothing else: "+
		`{"job_id":%q,"status":"completed","summary":"<one line describing what you did>"}`+"\n", job.JobID)
	sb.WriteString("\n<<context packet (authoritative; scope/constraints are not negotiable)>>\n")
	sb.Write(b)
	return sb.String()
}

// commitWorktree stages and commits all worktree changes onto the job branch so
// the result is durable and mergeable, using a harness identity so it never
// depends on the repo's git config. Returns the commit SHA, or an error if the
// stage/commit failed (review finding 2).
func commitWorktree(workdir, jobID string) (string, error) {
	if out, err := exec.Command("git", "-C", workdir, "add", "-A").CombinedOutput(); err != nil {
		return "", fmt.Errorf("git add: %s", out)
	}
	if out, err := exec.Command("git", "-C", workdir,
		"-c", "user.email=harness@local", "-c", "user.name=harness",
		"commit", "--allow-empty", "-q", "-m", "harness job "+jobID).CombinedOutput(); err != nil {
		return "", fmt.Errorf("git commit: %s", out)
	}
	sha, err := exec.Command("git", "-C", workdir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(sha)), nil
}

func (a *Adapter) writeArtifacts(jobID string, res runtime.Result) error {
	dir := a.L.Artifacts(a.SID, jobID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	_ = os.WriteFile(filepath.Join(dir, "stdout.log"), res.Stdout, 0o644)
	_ = os.WriteFile(filepath.Join(dir, "stderr.log"), res.Stderr, 0o644)
	_ = os.WriteFile(filepath.Join(dir, "final.json"), res.FinalJSON, 0o644)
	return store.WriteAtomicJSON(filepath.Join(dir, "process.json"), map[string]any{
		"exit_code":          res.ExitCode,
		"killed_by_watchdog": res.KilledByWatchdog,
		"final_json_ok":      res.FinalJSONOK,
		"reported_tokens":    res.ReportedTokens,
	})
}
