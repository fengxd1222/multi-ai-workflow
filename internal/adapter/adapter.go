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
	// integrate (rev3 §7). Commit whenever the job is a write job and the worktree
	// has changes — even on a torn result or non-zero exit — so a worker that
	// finished its edits before a network error cut the stream doesn't lose its
	// work (decide routes a dirty failure to needs-human). Done before scope
	// review so the diff sees it.
	if running.Writes && workdirDirty(workdir) {
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
		SchemaPath:    a.schemaPath(job),
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
	// Wire the in-session PreToolUse path guard so out-of-scope writes are blocked
	// before they happen, not just detected afterwards (rev3 §10; validated
	// against claude 2.1.156). The hook points back at the main repo's .harness.
	if job.TargetRuntime == model.RuntimeClaude && job.Writes {
		req.HookSettings = a.claudePreToolHook(job.JobID)
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
		// A process-layer failure (network/auth) may still have left real edits
		// (already committed above). Don't silently discard them — gate for review.
		if job.Writes && hasCommittedWork(workdir, base) {
			return model.JobNeedsHuman, fmt.Sprintf("runtime-exec-failed exit=%d but worktree has changes", res.ExitCode), nil
		}
		return model.JobFailed, fmt.Sprintf("runtime-exec-failed exit=%d", res.ExitCode), nil
	case !res.FinalJSONOK:
		if job.Writes && hasCommittedWork(workdir, base) {
			return model.JobNeedsHuman, "torn final.json but worktree has changes", nil
		}
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

	// Persist a read-only (analysis/research) job's findings so downstream
	// implement jobs can reference them as grounding (research→plan→delegate).
	if !job.Writes && r.Summary != "" {
		a.writeFindings(job, r.Summary)
	}

	a.recordUsage(actor, job.JobID, res.ReportedTokens, r)
	return model.JobCompleted, "ok", nil
}

// writeFindings stores an analysis job's summary as a markdown artifact a worker
// in any worktree can Read via the repo root.
func (a *Adapter) writeFindings(job model.Job, summary string) {
	path := a.L.Findings(a.SID, job.JobID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	body := fmt.Sprintf("# Findings — %s (%s)\n\nTask: %s\nGoal: %s\n\n%s\n",
		job.JobID, job.Role, job.TaskID, job.Goal, summary)
	_ = os.WriteFile(path, []byte(body), 0o644)
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
			SchemaPath:    a.schemaPath(job),
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

// schemaPath picks the output schema for a runtime. codex's --output-schema goes
// through OpenAI structured output, which requires every object to have
// additionalProperties:false AND every property in `required`; the full
// job-result schema (optional fields, a map-typed verification) violates that, so
// codex gets a minimal strict schema covering just the contract fields. claude's
// --json-schema is lenient and uses the full schema. (calibrated vs codex 0.135.0)
func (a *Adapter) schemaPath(job model.Job) string {
	f := "job-result.schema.json"
	if job.TargetRuntime == model.RuntimeCodex {
		f = "codex-output.schema.json"
	}
	return filepath.Join(a.L.Schemas(), f)
}

// buildPrompt renders the context packet as an actionable work order: a brief,
// hard constraints, files to Read first (so the worker grounds itself in real
// code instead of cold-starting), the scope boundary, and the required output
// contract. This is what turns "delegate a one-line goal" into "hand off a
// scoped ticket with grounding" (rev3 research→plan→delegate / context-packet).
func (a *Adapter) buildPrompt(job model.Job, workdir string) string {
	var sb strings.Builder
	sb.WriteString("You are a harness worker executing ONE scoped job. Stay within the boundaries below.\n\n")

	if job.Role != "" {
		fmt.Fprintf(&sb, "Role: %s\n", job.Role)
	}
	if job.Goal != "" {
		fmt.Fprintf(&sb, "Goal: %s\n", job.Goal)
	}
	if job.Brief != "" {
		fmt.Fprintf(&sb, "\nBrief:\n%s\n", job.Brief)
	}

	// Grounding: tell the worker to Read real files/findings BEFORE acting, so
	// cross-CLI understanding gaps are closed by evidence, not by guessing.
	if len(job.ContextRefs) > 0 {
		sb.WriteString("\nBefore doing anything, Read these files for context (paths are relative to the repo root):\n")
		for _, r := range job.ContextRefs {
			fmt.Fprintf(&sb, "  - %s\n", r)
		}
		fmt.Fprintf(&sb, "Repo root: %s\n", job.RepoRoot)
	}

	if len(job.Constraints) > 0 {
		sb.WriteString("\nHard constraints (do not violate):\n")
		for _, c := range job.Constraints {
			fmt.Fprintf(&sb, "  - %s\n", c)
		}
	}

	if job.Writes {
		fmt.Fprintf(&sb, "\nWrite scope: you may ONLY create/modify files within %s. Never write outside it (a guard will block you).\n",
			strings.Join(job.Scope.Allowed, ", "))
		if len(job.Scope.Denied) > 0 {
			fmt.Fprintf(&sb, "Explicitly forbidden: %s\n", strings.Join(job.Scope.Denied, ", "))
		}
	} else {
		sb.WriteString("\nThis is a READ-ONLY job. Do not modify any files; investigate and report.\n")
	}

	if len(job.VerificationRequirements) > 0 {
		sb.WriteString("\nYour work will be checked by the harness re-running these (self-reported success is ignored):\n")
		for _, v := range job.VerificationRequirements {
			fmt.Fprintf(&sb, "  - %s\n", v)
		}
	}

	if job.Writes {
		fmt.Fprintf(&sb, "\nWhen finished, output ONLY this JSON and nothing else:\n"+
			`{"job_id":%q,"status":"completed","summary":"<what you changed, one line>"}`+"\n", job.JobID)
	} else {
		fmt.Fprintf(&sb, "\nWhen finished, output ONLY this JSON and nothing else. Put your findings (facts, integration points, risks, file:line refs) in the summary:\n"+
			`{"job_id":%q,"status":"completed","summary":"<your findings>"}`+"\n", job.JobID)
	}
	return sb.String()
}

// claudePreToolHook builds a claude --settings JSON that routes PreToolUse
// events to `harness guard pretool` for this job. The command uses --repo to
// reach the main repo's .harness even though the worker runs in a worktree
// (rev3 N4). Empty if the harness binary path can't be resolved.
func (a *Adapter) claudePreToolHook(jobID string) string {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return ""
	}
	cmd := fmt.Sprintf("%s guard pretool --runtime claude --repo %s --session %s --job %s",
		shellQuote(exe), shellQuote(a.L.RepoRoot), a.SID, jobID)
	settings := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{map[string]any{
				"matcher": "Write|Edit|MultiEdit|NotebookEdit|Bash",
				"hooks":   []any{map[string]string{"type": "command", "command": cmd}},
			}},
		},
	}
	b, _ := json.Marshal(settings)
	return string(b)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// workdirDirty reports whether a worktree has any tracked-or-untracked changes.
func workdirDirty(workdir string) bool {
	out, err := exec.Command("git", "-C", workdir, "status", "--porcelain").Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}

// hasCommittedWork reports whether the worktree's HEAD has advanced past the
// job's base commit — i.e. the worker's edits were committed (used after the
// commit step, when the worktree itself is clean again).
func hasCommittedWork(workdir, base string) bool {
	if base == "" {
		return false
	}
	out, err := exec.Command("git", "-C", workdir, "rev-parse", "HEAD").Output()
	return err == nil && strings.TrimSpace(string(out)) != base
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
