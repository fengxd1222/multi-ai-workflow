package trellis

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNoInterpreter means no usable python interpreter was found and the script
// is not self-executable.
var ErrNoInterpreter = errors.New("no python interpreter found; set HARNESS_PYTHON")

func (p Project) scriptPath(name string) string {
	return filepath.Join(dir(p.Root), "scripts", name)
}

// HasScript reports whether a Trellis script (e.g. "add_session.py") exists.
func (p Project) HasScript(name string) bool {
	_, err := os.Stat(p.scriptPath(name))
	return err == nil
}

// RunScript runs a Trellis script with args from the repo root and returns
// combined output. Interpreter resolution (rev: python must not be hardcoded —
// it differs per machine):
//  1. $HARNESS_PYTHON (explicit; may be a venv/conda path or multi-word like
//     "conda run -n env python")
//  2. a self-executable script (shebang + exec bit) → run it directly
//  3. python3 → python → py on PATH, probed with --version
//
// Returns ErrNoInterpreter if none resolve, so callers can skip write-back
// gracefully instead of failing.
func (p Project) RunScript(name string, args ...string) (string, error) {
	script := p.scriptPath(name)
	if _, err := os.Stat(script); err != nil {
		return "", fmt.Errorf("trellis script %s not found", name)
	}
	argv, err := interpreterArgv(script)
	if err != nil {
		return "", err
	}
	full := append(argv[1:], args...)
	cmd := exec.Command(argv[0], full...)
	cmd.Dir = p.Root
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func interpreterArgv(script string) ([]string, error) {
	if v := strings.TrimSpace(os.Getenv("HARNESS_PYTHON")); v != "" {
		// supports "python", "/path/to/venv/bin/python", "conda run -n env python"
		return append(strings.Fields(v), script), nil
	}
	if isSelfExecutable(script) {
		return []string{script}, nil
	}
	for _, c := range []string{"python3", "python", "py"} {
		if interpreterWorks(c) {
			return []string{c, script}, nil
		}
	}
	return nil, ErrNoInterpreter
}

func isSelfExecutable(script string) bool {
	fi, err := os.Stat(script)
	if err != nil || fi.Mode()&0o111 == 0 {
		return false
	}
	f, err := os.Open(script)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 2)
	n, _ := f.Read(buf)
	return n == 2 && string(buf) == "#!"
}

func interpreterWorks(name string) bool {
	if _, err := exec.LookPath(name); err != nil {
		return false
	}
	return exec.Command(name, "--version").Run() == nil
}

// RecordSession appends a session entry to the developer's Trellis journal via
// add_session.py (parameters are documented; this is the only write-back wired
// in for now). commit may be empty.
func (p Project) RecordSession(title, commit, summary string) (string, error) {
	args := []string{"--title", title}
	if commit != "" {
		args = append(args, "--commit", commit)
	}
	if summary != "" {
		args = append(args, "--summary", summary)
	}
	return p.RunScript("add_session.py", args...)
}
