//go:build windows

package runtime

import (
	"os/exec"
	"strconv"
	"syscall"
)

// setProcessGroup starts the child as the root of a new process group so it is
// isolated from our console and can be torn down as a tree. Windows has no
// setpgid; CREATE_NEW_PROCESS_GROUP is the closest equivalent.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// killProcessGroup force-terminates the child and its whole descendant tree.
// `taskkill /F /T` is the Unix `kill -KILL -<pgid>` equivalent and ships with
// every Windows install, so we stay dependency-free. It is best-effort: if the
// tree already exited, taskkill simply reports nothing to kill.
func killProcessGroup(cmd *exec.Cmd) {
	_ = exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
}
