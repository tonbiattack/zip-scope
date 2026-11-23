//go:build windows

package app

import (
	"fmt"
	"os/exec"
	"syscall"
)

// makeDirLink creates a directory link at link pointing to target.
//
// A symbolic link needs SeCreateSymbolicLinkPrivilege, which a normal user does
// not have unless Developer Mode is on. A junction needs no privilege, so it is
// used instead: for the purpose of these tests both are reparse points that
// EvalSymlinks resolves.
func makeDirLink(target, link string) error {
	// mklink /J creates a junction without elevation.
	cmd := exec.Command("cmd", "/c", "mklink", "/J", link, target)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mklink /J: %v: %s", err, out)
	}
	return nil
}
