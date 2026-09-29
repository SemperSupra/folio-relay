//go:build windows

package state

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	lockfileFailImmediately = 0x00000001
	lockfileExclusiveLock   = 0x00000002
)

var (
	kernel32ProcLockFileEx   = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")
	kernel32ProcUnlockFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("UnlockFileEx")
)

func lockJournalFile(file *os.File) error {
	overlapped := new(syscall.Overlapped)
	r1, _, err := kernel32ProcLockFileEx.Call(
		uintptr(file.Fd()),
		lockfileExclusiveLock|lockfileFailImmediately,
		0,
		1,
		0,
		uintptr(unsafe.Pointer(overlapped)),
	)
	if r1 == 0 {
		if err != syscall.Errno(0) {
			return err
		}
		return syscall.EINVAL
	}
	return nil
}

func unlockJournalFile(file *os.File) error {
	overlapped := new(syscall.Overlapped)
	r1, _, err := kernel32ProcUnlockFileEx.Call(
		uintptr(file.Fd()),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(overlapped)),
	)
	if r1 == 0 {
		if err != syscall.Errno(0) {
			return err
		}
		return syscall.EINVAL
	}
	return nil
}
