//go:build !windows

package app

import "os"

// makeDirLink creates a symbolic link at link pointing to target.
func makeDirLink(target, link string) error {
	return os.Symlink(target, link)
}
