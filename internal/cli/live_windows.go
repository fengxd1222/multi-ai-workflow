//go:build windows

package cli

import (
	"syscall"
	"unsafe"
)

// Windows liveness probe via kernel32, called through a lazily-resolved DLL so
// the harness keeps ZERO external Go dependencies. OpenProcess succeeding only
// proves a handle could be opened (a not-yet-reaped pid can still open), so we
// confirm with GetExitCodeProcess: STILL_ACTIVE means genuinely running.
var (
	modkernel32            = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess        = modkernel32.NewProc("OpenProcess")
	procGetExitCodeProcess = modkernel32.NewProc("GetExitCodeProcess")
	procCloseHandle        = modkernel32.NewProc("CloseHandle")
)

const (
	processQueryLimitedInformation = 0x1000
	stillActive                    = 259 // STILL_ACTIVE
)

func pidLiveCheck(pid int) bool {
	h, _, _ := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(uint32(pid)))
	if h == 0 {
		return false // process does not exist (or no access at all)
	}
	defer procCloseHandle.Call(h)

	var code uint32
	r1, _, _ := procGetExitCodeProcess.Call(h, uintptr(unsafe.Pointer(&code)))
	if r1 == 0 {
		return true // opened but couldn't read exit code: assume alive
	}
	return code == stillActive
}
