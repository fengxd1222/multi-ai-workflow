//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"strings"
)

// bootID returns a host-boot identifier used to detect pid reuse across reboots
// (rev3 N1/N5). It is the anchor of stale-job detection: a recorded worker pid
// is only trusted if it was recorded under the current boot. Best-effort, but
// we try three sources before giving up so the guard rarely degrades to a bare
// pid check (which a post-reboot pid-reuse could fool):
//  1. /proc/sys/kernel/random/boot_id — Linux, a per-boot random UUID.
//  2. /proc/stat "btime" line          — Linux fallback when 1 is restricted.
//  3. sysctl kern.boottime             — macOS/BSD.
func bootID() string {
	if b, err := os.ReadFile("/proc/sys/kernel/random/boot_id"); err == nil {
		if id := strings.TrimSpace(string(b)); id != "" {
			return id
		}
	}
	if b, err := os.ReadFile("/proc/stat"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if rest, ok := strings.CutPrefix(line, "btime "); ok {
				if id := strings.TrimSpace(rest); id != "" {
					return "btime:" + id
				}
			}
		}
	}
	if out, err := exec.Command("sysctl", "-n", "kern.boottime").Output(); err == nil {
		if id := strings.TrimSpace(string(out)); id != "" {
			return id
		}
	}
	return ""
}
