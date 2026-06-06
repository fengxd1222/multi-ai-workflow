package adapter

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fengxd1222/multi-ai-workflow/internal/model"
	"github.com/fengxd1222/multi-ai-workflow/internal/runtime"
	"github.com/fengxd1222/multi-ai-workflow/internal/scope"
	"github.com/fengxd1222/multi-ai-workflow/internal/state"
	"github.com/fengxd1222/multi-ai-workflow/internal/store"
)

const sid = "S-1"

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "t@t.t"}, {"config", "user.name", "t"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "seed"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "seed"}, {"commit", "-qm", "base"}} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func newAdapter(t *testing.T, repo string, rt runtime.Runtime) (*Adapter, *state.Engine, store.Layout) {
	t.Helper()
	l := store.NewLayout(repo)
	if err := os.MkdirAll(l.Session(sid), 0o755); err != nil {
		t.Fatal(err)
	}
	eng := state.New(l, sid)
	return &Adapter{
		L: l, SID: sid, Eng: eng, RT: rt,
		Reserved: scope.Reserved{Patterns: []string{"**/.env"}},
		BootID:   "boot-1", MaxRepair: 1,
	}, eng, l
}

func mkJob(t *testing.T, eng *state.Engine, repo string, writes bool, sc model.Scope, verify []string) string {
	t.Helper()
	role := model.RoleImplementation
	if !writes {
		role = model.RoleAnalysis
	}
	j := model.Job{
		JobID: "J-1", TaskID: "T-1", CreatedBy: "codex", TargetRuntime: model.RuntimeClaude,
		Role: role, Writes: writes, Mode: model.ModeWorktree, StateRoot: repo + "/.harness",
		RepoRoot: repo, Workdir: repo, Scope: sc,
		VerificationRequirements: verify,
		Delegation:               model.Delegation{Depth: 1, ChainFingerprints: []string{"T-1:x"}},
		Budget:                   model.JobBudget{MaxTokens: 1000, TimeoutS: 2},
		ResultContract:           "job-result.schema.json",
	}
	if err := eng.CreateJob("codex", j); err != nil {
		t.Fatal(err)
	}
	return j.JobID
}

func goodResult() []byte {
	return []byte(`{"job_id":"J-1","status":"completed","summary":"done"}`)
}

func TestAdapter_Normal_Completed(t *testing.T) {
	repo := gitRepo(t)
	a, eng, _ := newAdapter(t, repo, runtime.Normal(goodResult(), 100))
	mkJob(t, eng, repo, true, model.Scope{Allowed: []string{"src/**"}}, nil)

	out, err := a.Run(context.Background(), "J-1")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != model.JobCompleted {
		t.Fatalf("status=%s reason=%s", out.Status, out.Reason)
	}
	if out.Job.Rev != 3 { // created(1) -> running(2) -> completed(3)
		t.Fatalf("rev=%d want 3", out.Job.Rev)
	}
}

// Ground-truth scope review must catch a write the worker did NOT report.
func TestAdapter_ScopeViolation_NeedsHuman(t *testing.T) {
	repo := gitRepo(t)
	// worker writes package.json (denied) but returns a clean-looking result.
	rt := runtime.ScopeViolation(goodResult(), map[string]string{"package.json": "{}"})
	a, eng, l := newAdapter(t, repo, rt)
	mkJob(t, eng, repo, true, model.Scope{Allowed: []string{"src/**"}, Denied: []string{"package.json"}}, nil)

	out, err := a.Run(context.Background(), "J-1")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != model.JobNeedsHuman {
		t.Fatalf("status=%s want needs-human (reason=%s)", out.Status, out.Reason)
	}
	if len(out.Violations) == 0 {
		t.Fatal("expected recorded violations")
	}
	// needs-human must open an actionable gate (review finding 1)
	if entries, _ := os.ReadDir(l.GatesDir(sid)); len(entries) == 0 {
		t.Fatal("needs-human did not open a gate")
	}
}

func TestCommitWorktree_ErrorOnNonGitDir(t *testing.T) {
	if _, err := commitWorktree(t.TempDir(), "J-1"); err == nil {
		t.Fatal("commitWorktree should error on a non-git dir (review finding 2)")
	}
}

func TestBuildPrompt_RichPacket(t *testing.T) {
	a := &Adapter{L: store.NewLayout("/repo"), SID: "S-1"}
	job := model.Job{
		JobID: "J-1", TaskID: "T-1", Role: model.RoleImplementation, Writes: true, RepoRoot: "/repo",
		Goal:        "refactor auth",
		Brief:       "auth lives in src/auth; keep API stable",
		ContextRefs: []string{".harness/sessions/S-1/findings/J-0.md", "src/auth/service.ts"},
		Constraints: []string{"do not change error codes", "no new deps"},
		Scope:       model.Scope{Allowed: []string{"src/auth/**"}, Denied: []string{"package.json"}},
		VerificationRequirements: []string{"npm test -- auth"},
	}
	p := a.buildPrompt(job, "/repo/.worktrees/J-1")
	for _, want := range []string{
		"refactor auth", "auth lives in src/auth", "Read these files",
		".harness/sessions/S-1/findings/J-0.md", "src/auth/service.ts",
		"do not change error codes", "src/auth/**", "package.json",
		"npm test -- auth", `"job_id":"J-1"`,
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q\n---\n%s", want, p)
		}
	}
}

func TestBuildPrompt_ReadOnlyAsksForFindings(t *testing.T) {
	a := &Adapter{L: store.NewLayout("/repo"), SID: "S-1"}
	p := a.buildPrompt(model.Job{JobID: "J-1", Role: model.RoleAnalysis, Writes: false, Goal: "survey auth"}, "/repo")
	if !strings.Contains(p, "READ-ONLY") || !strings.Contains(p, "findings") {
		t.Fatalf("read-only prompt should ask for findings:\n%s", p)
	}
	if strings.Contains(p, "Write scope") {
		t.Fatal("read-only prompt must not grant write scope")
	}
}

func TestWriteFindings(t *testing.T) {
	repo := gitRepo(t)
	a, _, l := newAdapter(t, repo, runtime.Normal(goodResult(), 1))
	job := model.Job{JobID: "J-9", TaskID: "T-1", Role: model.RoleAnalysis, Goal: "survey"}
	a.writeFindings(job, "auth uses bcrypt; risk: token rotation")
	data, err := os.ReadFile(l.Findings(sid, "J-9"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "auth uses bcrypt") || !strings.Contains(string(data), "J-9") {
		t.Fatalf("findings file wrong:\n%s", data)
	}
}

func TestClaudePreToolHook_Settings(t *testing.T) {
	a := &Adapter{L: store.NewLayout("/repo"), SID: "S-1"}
	s := a.claudePreToolHook("J-1")
	if s == "" {
		t.Skip("os.Executable() unavailable")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("hook settings not valid JSON: %v\n%s", err, s)
	}
	for _, want := range []string{"PreToolUse", "guard pretool", "--repo", "/repo", "--session S-1", "--job J-1"} {
		if !strings.Contains(s, want) {
			t.Errorf("hook settings missing %q in: %s", want, s)
		}
	}
}

func TestAdapter_BadSchema_RepairsThenCompletes(t *testing.T) {
	repo := gitRepo(t)
	calls := 0
	rt := &runtime.Mock{Fn: func(_ context.Context, _ runtime.Request) (runtime.Result, error) {
		calls++
		if calls == 1 {
			return runtime.Result{ExitCode: 0, FinalJSON: []byte(`{"bad":1}`), FinalJSONOK: true}, nil
		}
		return runtime.Result{ExitCode: 0, FinalJSON: goodResult(), FinalJSONOK: true}, nil
	}}
	a, eng, _ := newAdapter(t, repo, rt)
	mkJob(t, eng, repo, true, model.Scope{Allowed: []string{"src/**"}}, nil)

	out, err := a.Run(context.Background(), "J-1")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != model.JobCompleted || calls != 2 {
		t.Fatalf("status=%s calls=%d want completed/2", out.Status, calls)
	}
}

func TestAdapter_VerifyFails(t *testing.T) {
	repo := gitRepo(t)
	a, eng, _ := newAdapter(t, repo, runtime.Normal(goodResult(), 1))
	mkJob(t, eng, repo, true, model.Scope{Allowed: []string{"src/**"}}, []string{"false"})

	out, err := a.Run(context.Background(), "J-1")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != model.JobFailed || out.Reason != "verify-failed" {
		t.Fatalf("status=%s reason=%s want failed/verify-failed", out.Status, out.Reason)
	}
}

func TestAdapter_NonZeroExit_Failed(t *testing.T) {
	repo := gitRepo(t)
	a, eng, _ := newAdapter(t, repo, runtime.NonZeroExit(40))
	mkJob(t, eng, repo, true, model.Scope{Allowed: []string{"src/**"}}, nil)
	out, _ := a.Run(context.Background(), "J-1")
	if out.Status != model.JobFailed {
		t.Fatalf("status=%s want failed", out.Status)
	}
}

func TestAdapter_TornFinalJSON_Failed(t *testing.T) {
	repo := gitRepo(t)
	a, eng, _ := newAdapter(t, repo, runtime.TornFinalJSON())
	mkJob(t, eng, repo, true, model.Scope{Allowed: []string{"src/**"}}, nil)
	out, _ := a.Run(context.Background(), "J-1")
	if out.Status != model.JobFailed {
		t.Fatalf("status=%s want failed (torn, no changes)", out.Status)
	}
}

// A torn response AFTER the worker made edits must preserve the work: commit it
// to the job branch and route to needs-human, not discard it.
func TestAdapter_TornButWroteFiles_NeedsHumanAndPreserved(t *testing.T) {
	repo := gitRepo(t)
	a, eng, _ := newAdapter(t, repo, runtime.WroteThenTorn(map[string]string{"src/new.ts": "export const x=1\n"}))
	mkJob(t, eng, repo, true, model.Scope{Allowed: []string{"src/**"}}, nil)

	out, err := a.Run(context.Background(), "J-1")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != model.JobNeedsHuman {
		t.Fatalf("status=%s want needs-human (torn but dirty)", out.Status)
	}
	// the worker's edit must be committed on the job branch (not lost)
	o, e := exec.Command("git", "-C", repo, "show", "job/J-1:src/new.ts").Output()
	if e != nil || string(o) != "export const x=1\n" {
		t.Fatalf("worker's work was lost: err=%v out=%q", e, o)
	}
}

func TestAdapter_Zombie_Timeout(t *testing.T) {
	repo := gitRepo(t)
	a, eng, _ := newAdapter(t, repo, runtime.Zombie())
	mkJob(t, eng, repo, true, model.Scope{Allowed: []string{"src/**"}}, nil) // TimeoutS=2
	out, err := a.Run(context.Background(), "J-1")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != model.JobTimeout {
		t.Fatalf("status=%s want timeout", out.Status)
	}
}

func TestAdapter_ArtifactsWritten(t *testing.T) {
	repo := gitRepo(t)
	a, eng, l := newAdapter(t, repo, runtime.Normal(goodResult(), 1))
	mkJob(t, eng, repo, true, model.Scope{Allowed: []string{"src/**"}}, nil)
	if _, err := a.Run(context.Background(), "J-1"); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"final.json", "process.json", "stdout.log"} {
		if _, err := os.Stat(filepath.Join(l.Artifacts(sid, "J-1"), f)); err != nil {
			t.Errorf("artifact %s missing: %v", f, err)
		}
	}
}
