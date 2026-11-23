//go:build !windows

package app

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// isCrossDevice reports whether a rename failed only because the source and
// destination are on different filesystems.
func isCrossDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

// publishNoReplace moves src to dest, failing if dest already exists.
//
// On POSIX systems link(2) fails when the destination exists, which gives the
// same no-overwrite guarantee as MoveFileExW on Windows. The source is removed
// afterwards so the result is a move rather than a second name for the file.
func publishNoReplace(src, dest string) error {
	if err := os.Link(src, dest); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrTargetExists
		}
		return fmt.Errorf("app: publish %s: %w", dest, err)
	}
	if err := os.Remove(src); err != nil {
		// The destination is in place; leaving the temporary file behind is
		// harmless and the caller cleans it up.
		return nil
	}
	return nil
}
