package cli

// processAlive reports whether a recorded worker process is still running
// (rev3 §15 N5). A boot_id mismatch means the pid is from a previous boot and
// could be reused, so it is treated as dead. The actual liveness probe is
// platform-specific (live_unix.go: kill(pid,0); live_windows.go: OpenProcess).
func processAlive(pid int, recordedBoot, currentBoot string) bool {
	if recordedBoot != "" && currentBoot != "" && recordedBoot != currentBoot {
		return false
	}
	if pid <= 0 {
		return false
	}
	return pidLiveCheck(pid)
}
