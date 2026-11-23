//go:build windows

package app

import (
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	procGetFinalPathNameByHandleW = kernel32.NewProc("GetFinalPathNameByHandleW")
)

// volumeNamePrefix is what GetFinalPathNameByHandleW puts in front of a path.
const volumeNamePrefix = `\\?\`

// filepathAbs is a thin alias so the platform files share one spelling.
func filepathAbs(path string) (string, error) { return filepath.Abs(path) }

// resolveRealPath returns the canonical path with every reparse point (symbolic
// link, junction, mount point) resolved.
//
// filepath.EvalSymlinks does not resolve junctions on Windows, so the path is
// opened and asked for its final name instead. That is the same resolution the
// filesystem itself performs, which is what a containment check needs.
func resolveRealPath(path string) (string, error) {
	abs, err := filepathAbs(path)
	if err != nil {
		return "", err
	}

	// FILE_FLAG_BACKUP_SEMANTICS allows opening a directory.
	const (
		fileReadAttributes = 0x0080
		fileShareAll       = 0x00000007
		openExisting       = 3
		backupSemantics    = 0x02000000
	)

	p, err := syscall.UTF16PtrFromString(abs)
	if err != nil {
		return "", fmt.Errorf("app: encode path: %w", err)
	}
	h, err := syscall.CreateFile(p, fileReadAttributes, fileShareAll, nil, openExisting,
		backupSemantics, 0)
	if err != nil {
		return "", err
	}
	defer syscall.CloseHandle(h)

	buf := make([]uint16, syscall.MAX_LONG_PATH)
	n, _, callErr := procGetFinalPathNameByHandleW.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		0, // FILE_NAME_NORMALIZED | VOLUME_NAME_DOS
	)
	if n == 0 {
		return "", fmt.Errorf("app: resolve %s: %w", abs, callErr)
	}
	if int(n) > len(buf) {
		return "", fmt.Errorf("app: resolve %s: path too long", abs)
	}

	final := syscall.UTF16ToString(buf[:n])
	// Strip the \\?\ prefix so the result compares equal to a normal path.
	if len(final) > len(volumeNamePrefix) && final[:len(volumeNamePrefix)] == volumeNamePrefix {
		final = final[len(volumeNamePrefix):]
	}
	return final, nil
}
