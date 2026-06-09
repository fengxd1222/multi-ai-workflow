//go:build windows

package store

import (
	"os"
	"syscall"
	"unsafe"
)

// Windows file locking via kernel32 LockFileEx / UnlockFileEx, called through a
// lazily-resolved DLL so the harness keeps ZERO external Go dependencies (no
// golang.org/x/sys). Semantics match flock(2): the lock is mandatory for the
// byte range we take (the whole file) and is released when the handle closes,
// including on process death — so a crash never strands the lock.
var (
	modkernel32      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = modkernel32.NewProc("LockFileEx")
	procUnlockFileEx = modkernel32.NewProc("UnlockFileEx")
)

const (
	lockfileFailImmediately = 0x00000001
	lockfileExclusiveLock   = 0x00000002
	// ERROR_LOCK_VIOLATION: a non-blocking lock attempt hit a held lock.
	errLockViolation = syscall.Errno(33)
)

// overlapped is the minimal OVERLAPPED LockFileEx requires; Offset/OffsetHigh
// select where the lock starts (0). The whole file is locked by passing a
// max-DWORD length, which is the conventional "lock everything" sentinel.
type overlapped struct {
	internal     uintptr
	internalHigh uintptr
	offset       uint32
	offsetHigh   uint32
	hEvent       syscall.Handle
}

func lockFile(f *os.File, nonblock bool) error {
	flags := uintptr(lockfileExclusiveLock)
	if nonblock {
		flags |= lockfileFailImmediately
	}
	var ol overlapped
	r1, _, e := procLockFileEx.Call(
		f.Fd(), flags, 0,
		0xFFFFFFFF, 0xFFFFFFFF, // lock the entire file
		uintptr(unsafe.Pointer(&ol)),
	)
	if r1 == 0 {
		if nonblock {
			if errno, ok := e.(syscall.Errno); ok && errno == errLockViolation {
				return errWouldBlock
			}
		}
		return e
	}
	return nil
}

func unlockFile(f *os.File) error {
	var ol overlapped
	r1, _, e := procUnlockFileEx.Call(
		f.Fd(), 0,
		0xFFFFFFFF, 0xFFFFFFFF,
		uintptr(unsafe.Pointer(&ol)),
	)
	if r1 == 0 {
		return e
	}
	return nil
}
