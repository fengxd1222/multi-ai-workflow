//go:build windows

package cli

import (
	"strconv"
	"time"
)

// modkernel32 is declared in live_windows.go (same package, same build tag).
var procGetTickCount64 = modkernel32.NewProc("GetTickCount64")

// bootID derives a per-boot identifier on Windows, which has no boot_id file.
// GetTickCount64 gives milliseconds since boot; subtracting it from the current
// wall clock yields the approximate boot instant, which is stable within a boot
// and jumps by the whole uptime across a reboot. We bucket to 10s so sub-second
// clock jitter does not flip the value mid-session, while a reboot still moves
// it unmistakably — exactly what pid-reuse detection (N1/N5) needs.
func bootID() string {
	r1, _, _ := procGetTickCount64.Call()
	uptimeMS := int64(r1)
	if uptimeMS <= 0 {
		return ""
	}
	bootMS := time.Now().UnixMilli() - uptimeMS
	bucket := bootMS / 10000 // 10-second bucket
	return "wintick:" + strconv.FormatInt(bucket, 10)
}
