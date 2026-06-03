package runtime

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunProcess_CaptureAndExit(t *testing.T) {
	pr := runProcess(context.Background(), t.TempDir(), "sh", []string{"-c", "printf hi; exit 0"})
	if pr.ExitCode != 0 || string(pr.Stdout) != "hi" || pr.Killed {
		t.Fatalf("unexpected: %+v", pr)
	}
	pr = runProcess(context.Background(), t.TempDir(), "sh", []string{"-c", "exit 3"})
	if pr.ExitCode != 3 {
		t.Fatalf("exit = %d want 3", pr.ExitCode)
	}
}

func TestRunProcess_WatchdogKillsGroup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	pr := runProcess(ctx, t.TempDir(), "sh", []string{"-c", "sleep 10"})
	if !pr.Killed {
		t.Fatal("watchdog should have killed the process")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("watchdog kill was too slow")
	}
}

func TestCompleteJSON(t *testing.T) {
	if !completeJSON([]byte(`{"a":1}`)) {
		t.Error("complete JSON should pass")
	}
	if completeJSON([]byte(`{"a":`)) {
		t.Error("torn JSON should fail")
	}
	if completeJSON([]byte("   ")) {
		t.Error("blank should fail")
	}
}

func TestCodexArgs(t *testing.T) {
	args := codexArgs(Request{Workdir: "/wt", Prompt: "do it", SchemaPath: "/s.json", Sandbox: "workspace-write"}, "/final.json")
	joined := strings.Join(args, " ")
	for _, want := range []string{"exec", "--json", "--sandbox workspace-write", "--output-schema /s.json", "--output-last-message /final.json", "-C /wt", "do it"} {
		if !strings.Contains(joined, want) {
			t.Errorf("codex args missing %q in: %s", want, joined)
		}
	}
}

func TestClaudeArgs(t *testing.T) {
	args := claudeArgs(Request{Prompt: "do it", AllowedTools: []string{"Read", "Edit"}}, `{"type":"object"}`)
	joined := strings.Join(args, " ")
	for _, want := range []string{"-p", "do it", "--output-format json", "--json-schema", "--allowedTools Read,Edit", "--permission-mode acceptEdits"} {
		if !strings.Contains(joined, want) {
			t.Errorf("claude args missing %q in: %s", want, joined)
		}
	}
}

func TestCodex_Run_MissingBinIsProcessFailure(t *testing.T) {
	r, err := Codex{Bin: "harness-no-such-bin"}.Run(context.Background(), Request{Workdir: t.TempDir()})
	if err != nil {
		t.Fatalf("Run should not hard-error on missing bin: %v", err)
	}
	if r.ExitCode != -1 || r.FinalJSONOK {
		t.Fatalf("missing bin should be exit -1, final not ok: %+v", r)
	}
}

func TestClaude_Run_MissingBinIsProcessFailure(t *testing.T) {
	r, err := Claude{Bin: "harness-no-such-bin"}.Run(context.Background(), Request{Workdir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if r.ExitCode != -1 || r.FinalJSONOK {
		t.Fatalf("missing bin should be exit -1, final not ok: %+v", r)
	}
}
