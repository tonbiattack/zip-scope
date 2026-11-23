//go:build !windows

package app

import "path/filepath"

// resolveRealPath returns the canonical path with every symbolic link resolved.
func resolveRealPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
