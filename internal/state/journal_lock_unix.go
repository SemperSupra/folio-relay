//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package state

import (
	"os"
	"syscall"
)

func lockJournalFile(file *os.File) error {
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return err
	}
	return nil
}

func unlockJournalFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
