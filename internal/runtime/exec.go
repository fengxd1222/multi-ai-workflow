package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"syscall"
)

// procResult is the raw process-layer outcome.
type procResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	Killed   bool // SIGKILL'd because the context (watchdog) fired
}

// runProcess runs name+args in dir in its own process group and, if the context
// is cancelled (the adapter watchdog firing past TimeoutS), SIGKILLs the whole
// group so a zombie child cannot outlive its job (rev3 §5 N3).
func runProcess(ctx context.Context, dir, name string, args []string) procResult {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Explicit empty stdin: codex/claude otherwise block on "Reading additional
	// input from stdin..." instead of getting an immediate EOF (calibrated
	// against codex-cli 0.135.0).
	cmd.Stdin = bytes.NewReader(nil)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return procResult{Stderr: []byte(err.Error()), ExitCode: -1}
	}

	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()

	killed := false
	select {
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // negative pid = process group
		}
		killed = true
		<-done // reap
	case <-done:
	}

	exit := -1
	if cmd.ProcessState != nil {
		exit = cmd.ProcessState.ExitCode()
	}
	return procResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), ExitCode: exit, Killed: killed}
}

// completeJSON reports whether b is a complete, parseable JSON document — the
// torn-final.json check that routes incomplete output to runtime-exec-failed
// rather than result-invalid (rev3 §8.2 N36).
func completeJSON(b []byte) bool {
	return len(bytes.TrimSpace(b)) > 0 && json.Valid(b)
}
