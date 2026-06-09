package store

import (
	"errors"
	"os"
	"path/filepath"
)

// FileLock is an advisory whole-file lock held for the duration of a critical
// section. state.lock serializes all state-transition commits; recover.lock
// makes `harness recover` mutually exclusive (rev3 §4, §15, fixes N6). The
// lock is released when the underlying handle is closed — including on process
// death — so a crash never strands the lock.
//
// The OS primitive is platform-specific: flock(2) on Unix (lock_unix.go),
// LockFileEx on Windows (lock_windows.go). This file holds the shared,
// OS-independent lifecycle; the platform files supply lockFile/unlockFile.
type FileLock struct {
	f *os.File
}

// errWouldBlock is returned by lockFile when a non-blocking lock attempt finds
// the lock already held by someone else.
var errWouldBlock = errors.New("lock would block")

// AcquireLock blocks until it holds an exclusive lock on path.
func AcquireLock(path string) (*FileLock, error) {
	f, err := openLockFile(path)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f, false); err != nil {
		f.Close()
		return nil, err
	}
	return &FileLock{f: f}, nil
}

// TryLock attempts a non-blocking exclusive lock. ok=false means another holder
// has it (used by recover.lock to bail early, rev3 N6).
func TryLock(path string) (lock *FileLock, ok bool, err error) {
	f, err := openLockFile(path)
	if err != nil {
		return nil, false, err
	}
	if err := lockFile(f, true); err != nil {
		f.Close()
		if errors.Is(err, errWouldBlock) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &FileLock{f: f}, true, nil
}

// Release unlocks and closes the lock file.
func (l *FileLock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = unlockFile(l.f)
	err := l.f.Close()
	l.f = nil
	return err
}

func openLockFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
}
