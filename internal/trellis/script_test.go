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
	out, err := Project{Root: root}.RecordSession("a title", "abc123", "a summary", "job/J-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--title", "a title", "--no-commit", "--commit", "abc123", "--summary", "a summary", "--branch", "job/J-1"} {
		if !strings.Contains(out, want) {
			t.Errorf("RecordSession args missing %q in %q", want, out)
		}
	}
}

func TestRecordSession_OmitsEmptyCommit(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "add_session.py", "#!/bin/sh\necho \"$*\"\n", true)
	out, _ := Project{Root: root}.RecordSession("t", "", "s", "")
	if strings.Contains(out, "--commit") || strings.Contains(out, "--branch") {
		t.Fatalf("empty commit/branch should be omitted: %q", out)
	}
	if !strings.Contains(out, "--no-commit") {
		t.Fatalf("--no-commit should always be present: %q", out)
	}
}

func TestCurrentTask(t *testing.T) {
	root := t.TempDir()
	// no task.py -> not ok
	if _, ok := (Project{Root: root}).CurrentTask(); ok {
		t.Fatal("no task.py should be not-ok")
	}
	// active task: stub task.py that prints a path on `current`
	writeScript(t, root, "task.py", "#!/bin/sh\n[ \"$1\" = current ] && echo '.trellis/tasks/05-12-xterm/' && exit 0\nexit 1\n", true)
	slug, ok := Project{Root: root}.CurrentTask()
	if !ok || slug != "05-12-xterm" {
		t.Fatalf("CurrentTask = %q,%v want 05-12-xterm,true", slug, ok)
	}
	// none active: exit 1, empty stdout -> not ok
	writeScript(t, root, "task.py", "#!/bin/sh\nexit 1\n", true)
	if _, ok := (Project{Root: root}).CurrentTask(); ok {
		t.Fatal("exit 1 should be not-ok")
	}
}

func TestSetBranch(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "task.py", "#!/bin/sh\necho \"$*\"\n", true)
	out, err := Project{Root: root}.SetBranch("05-12-xterm", "job/J-1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "set-branch 05-12-xterm job/J-1") {
		t.Fatalf("SetBranch args = %q", out)
	}
}
