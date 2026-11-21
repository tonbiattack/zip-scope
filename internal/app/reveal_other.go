//go:build !windows

package app

import "errors"

// revealPath is only implemented on Windows; the app targets Windows 11 x64.
func revealPath(path string) error {
	return errors.New("app: reveal is only supported on Windows")
}
