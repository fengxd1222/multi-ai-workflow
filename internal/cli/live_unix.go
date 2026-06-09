//go:build !windows

package cli

import "syscall"

// pidLiveCheck probes liveness without signalling: kill(pid,0). EPERM means the
// process exists but is owned by another user — still alive for our purposes.
func pidLiveCheck(pid int) bool {
	err := syscall.Kill(pid, syscall.Signal(0))
	return err == nil || err == syscall.EPERM
}
