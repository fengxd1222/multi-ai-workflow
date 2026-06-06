package trellis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeScript(t *testing.T, root, name, body string, exec bool) {
	t.Helper()
	sd := filepath.Join(root, ".trellis", "scripts")
	if err := os.MkdirAll(sd, 0o755); err != nil {
		t.Fatal(err)
	}
	mode := os.FileMode(0o644)
	if exec {
		mode = 0o755
	}
	if err := os.WriteFile(filepath.Join(sd, name), []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func TestHasScript(t *testing.T) {
	root := t.TempDir()
	p := Project{Root: root}
	if p.HasScript("task.py") {
		t.Fatal("should not have script yet")
	}
	writeScript(t, root, "task.py", "x", false)
	if !p.HasScript("task.py") {
		t.Fatal("should have script now")
	}
}

// TestRunScript_SelfExecutable verifies the shebang+exec-bit path: harness runs
// the script directly without choosing an interpreter.
func TestRunScript_SelfExecutable(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "echo.py", "#!/bin/sh\necho \"got:$*\"\n", true)
	out, err := Project{Root: root}.RunScript("echo.py", "--title", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "got:--title hello") {
		t.Fatalf("self-exec output = %q", out)
	}
}

// TestRunScript_ExplicitInterpreter verifies $HARNESS_PYTHON override (using
// /bin/sh as a stand-in interpreter so the test needs no python).
func TestRunScript_ExplicitInterpreter(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "echo.py", "echo \"sh-ran:$*\"\n", false) // not executable
	t.Setenv("HARNESS_PYTHON", "/bin/sh")
	out, err := Project{Root: root}.RunScript("echo.py", "a", "b")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "sh-ran:a b") {
		t.Fatalf("explicit-interpreter output = %q", out)
	}
}

func TestRunScript_MissingScript(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".trellis", "scripts"), 0o755)
	p := Project{Root: root}
	if _, err := p.RunScript("nope.py"); err == nil {
		t.Fatal("missing script should error")
	}
}

func TestRecordSession_SelfExec(t *testing.T) {
	root := t.TempDir()
	// fake add_session.py that echoes its args so we can assert wiring
	writeScript(t, root, "add_session.py", "#!/bin/sh\necho \"$*\"\n", true)
	out, err := Project{Root: root}.RecordSession("a title", "abc123", "a summary")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--title", "a title", "--commit", "abc123", "--summary", "a summary"} {
		if !strings.Contains(out, want) {
			t.Errorf("RecordSession args missing %q in %q", want, out)
		}
	}
}

func TestRecordSession_OmitsEmptyCommit(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "add_session.py", "#!/bin/sh\necho \"$*\"\n", true)
	out, _ := Project{Root: root}.RecordSession("t", "", "s")
	if strings.Contains(out, "--commit") {
		t.Fatalf("empty commit should be omitted: %q", out)
	}
}
