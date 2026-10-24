//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly && !windows

package state

import (
	"fmt"
	"os"
	"runtime"
)

func lockJournalFile(file *os.File) error {
	return fmt.Errorf("journal writer locking is unsupported on %s", runtime.GOOS)
}

func unlockJournalFile(file *os.File) error {
	return nil
}
