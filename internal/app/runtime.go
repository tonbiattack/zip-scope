package app

import (
	"errors"
	"os"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// osStat is a thin wrapper so tests can reason about the same call the app
// makes.
func osStat(path string) (os.FileInfo, error) { return os.Stat(path) }

// isPermission reports whether err is an access-denied style failure.
func isPermission(err error) bool {
	return errors.Is(err, os.ErrPermission)
}

// emit sends an event to the frontend. It is a no-op before Startup, which
// keeps unit tests free of a Wails runtime.
func (a *App) emit(name string, payload any) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, name, payload)
}

// pickFolder shows the native folder picker.
func (a *App) pickFolder() (string, error) {
	if a.ctx == nil {
		return "", errors.New("app: runtime not ready")
	}
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "ZIP を探すフォルダを選択",
	})
}

// pickArchive shows the native file picker for one archive.
func (a *App) pickArchive() (string, error) {
	if a.ctx == nil {
		return "", errors.New("app: runtime not ready")
	}
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "ZIP / CBZ を選択",
		Filters: []runtime.FileFilter{
			{DisplayName: "ZIP / CBZ", Pattern: "*.zip;*.cbz"},
			{DisplayName: "すべてのファイル", Pattern: "*.*"},
		},
	})
}
