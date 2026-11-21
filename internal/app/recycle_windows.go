//go:build windows

package app

import (
	"fmt"
	"syscall"
	"unsafe"
)

// SHFileOperationW flags.
const (
	foDelete        = 0x0003
	foAllowUndo     = 0x0040
	foNoConfirmation = 0x0010
	foSilent        = 0x0004
	foNoErrorUI     = 0x0400
)

// shFileOpStructW mirrors the Win32 SHFILEOPSTRUCTW layout. The pFrom field is
// a double-null-terminated list of paths.
type shFileOpStructW struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

var (
	shell32            = syscall.NewLazyDLL("shell32.dll")
	procSHFileOperationW = shell32.NewProc("SHFileOperationW")
)

// recycleSupported reports whether the Recycle Bin API is available. It always
// is on Windows, but the check keeps the capability honest.
func recycleSupported() bool {
	return procSHFileOperationW.Find() == nil
}

// recycleFile sends path to the Recycle Bin. The operation is undoable, which
// is why it is the default delete action.
func recycleFile(path string) error {
	// SHFileOperationW requires a double-null-terminated path list.
	from, err := syscall.UTF16FromString(path)
	if err != nil {
		return fmt.Errorf("app: encode path: %w", err)
	}
	from = append(from, 0)

	op := shFileOpStructW{
		wFunc:  foDelete,
		pFrom:  &from[0],
		fFlags: foAllowUndo | foNoConfirmation | foSilent | foNoErrorUI,
	}
	ret, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	if ret != 0 {
		return fmt.Errorf("app: recycle %s: SHFileOperation failed with code %d", path, ret)
	}
	if op.fAnyOperationsAborted != 0 {
		return fmt.Errorf("app: recycle %s: operation was aborted", path)
	}
	return nil
}
