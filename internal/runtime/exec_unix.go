//go:build !windows

package runtime

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the child in its own process group so a watchdog kill
// can signal the whole group (the CLI plus any helpers it spawns).
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup SIGKILLs the entire group via the negative pid, so a zombie
// child cannot outlive its job when the adapter watchdog fires (rev3 §5 N3).
func killProcessGroup(cmd *exec.Cmd) {
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // negative pid = process group
}
