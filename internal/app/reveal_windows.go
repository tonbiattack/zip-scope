//go:build windows

package app

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
)

// revealPath opens Explorer with the given file selected.
func revealPath(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	// explorer.exe returns a non-zero exit code even on success, so the exit
	// status is deliberately ignored.
	cmd := exec.Command("explorer.exe", "/select,"+abs)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("app: reveal: %w", err)
	}
	go cmd.Wait()
	return nil
}
