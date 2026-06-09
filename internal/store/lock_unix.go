//go:build !windows

package store

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive advisory lock on f via flock(2). When nonblock is
// true and the lock is already held, it returns errWouldBlock instead of waiting.
func lockFile(f *os.File, nonblock bool) error {
	how := syscall.LOCK_EX
	if nonblock {
		how |= syscall.LOCK_NB
	}
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		if err == syscall.EWOULDBLOCK {
			return errWouldBlock
		}
		return err
	}
	return nil
}

// unlockFile releases the flock. Closing the fd would release it too, but we
// unlock explicitly so a reused *os.File never carries a stale lock.
func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
