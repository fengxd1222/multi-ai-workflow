package store

import (
	"os"
	"path/filepath"
	"syscall"
)

// FileLock is an advisory flock(2) held for the duration of a critical section.
// state.lock serializes all state-transition commits; recover.lock makes
// `harness recover` mutually exclusive (rev3 §4, §15, fixes N6).
type FileLock struct {
	f *os.File
}

// AcquireLock blocks until it holds an exclusive lock on path.
func AcquireLock(path string) (*FileLock, error) {
	f, err := openLockFile(path)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
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
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if err == syscall.EWOULDBLOCK {
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
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
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
