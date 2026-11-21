//go:build !windows

package app

import "errors"

// recycleSupported reports that the Recycle Bin is unavailable off Windows.
func recycleSupported() bool { return false }

// recycleFile is not implemented off Windows; the app targets Windows 11 x64.
func recycleFile(path string) error {
	return errors.New("app: recycle bin is only supported on Windows")
}
