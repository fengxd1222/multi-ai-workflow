package cli

import "syscall"

// processAlive reports whether a recorded worker process is still running
// (rev3 §15 N5). A boot_id mismatch means the pid is from a previous boot and
// could be reused, so it is treated as dead. kill(pid,0) probes liveness without
// signalling; EPERM means the process exists but we lack permission.
func processAlive(pid int, recordedBoot, currentBoot string) bool {
	if recordedBoot != "" && currentBoot != "" && recordedBoot != currentBoot {
		return false
	}
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, syscall.Signal(0))
	return err == nil || err == syscall.EPERM
}
