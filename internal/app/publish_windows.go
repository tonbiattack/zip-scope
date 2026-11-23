//go:build windows

package app

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procMoveFileExW  = kernel32.NewProc("MoveFileExW")
)

// MOVEFILE_WRITE_THROUGH flushes the move to disk before returning.
const movefileWriteThrough = 0x00000008

// errorNotSameDevice is ERROR_NOT_SAME_DEVICE, which a rename returns when the
// source and destination are on different volumes. syscall does not export it.
const errorNotSameDevice syscall.Errno = 17

// isCrossDevice reports whether a rename failed only because the source and
// destination are on different volumes. That is the one failure a copy can
// solve; permission and lock errors would fail the copy too.
func isCrossDevice(err error) bool {
	return errors.Is(err, errorNotSameDevice)
}

// publishNoReplace moves src to dest, failing if dest already exists.
//
// os.Rename on Windows replaces an existing destination, which would silently
// destroy a file that appeared between the caller's existence check and this
// call. MoveFileExW without MOVEFILE_REPLACE_EXISTING fails with
// ERROR_ALREADY_EXISTS instead, which is the behaviour a move needs.
func publishNoReplace(src, dest string) error {
	from, err := syscall.UTF16PtrFromString(src)
	if err != nil {
		return fmt.Errorf("app: encode source path: %w", err)
	}
	to, err := syscall.UTF16PtrFromString(dest)
	if err != nil {
		return fmt.Errorf("app: encode destination path: %w", err)
	}

	r, _, callErr := procMoveFileExW.Call(
		uintptr(unsafe.Pointer(from)),
		uintptr(unsafe.Pointer(to)),
		uintptr(movefileWriteThrough),
	)
	if r != 0 {
		return nil
	}
	if errors.Is(callErr, syscall.ERROR_ALREADY_EXISTS) || errors.Is(callErr, syscall.ERROR_FILE_EXISTS) {
		return ErrTargetExists
	}
	return fmt.Errorf("app: publish %s: %w", dest, callErr)
}
