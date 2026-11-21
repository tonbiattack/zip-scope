package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tonbiattack/zip-scope/internal/store"
)

// Organize errors. They are distinct so the UI can explain exactly what went
// wrong instead of showing a generic failure.
var (
	// ErrTargetExists reports that the destination already has a file.
	ErrTargetExists = errors.New("app: destination already exists")
	// ErrSamePath reports a move onto itself.
	ErrSamePath = errors.New("app: source and destination are the same")
	// ErrNotArchive reports a path that is not a ZIP/CBZ.
	ErrNotArchive = errors.New("app: not a zip or cbz file")
	// ErrOutsideRoot reports a destination outside the scanned root.
	ErrOutsideRoot = errors.New("app: destination is outside the scanned root")
	// ErrArchiveOpen reports that the archive is still in use.
	ErrArchiveOpen = errors.New("app: archive is still open")
)

// OrganizeResult reports what an organize operation did.
type OrganizeResult struct {
	// Source is the original path.
	Source string `json:"source"`
	// Destination is the new path, or empty for a delete.
	Destination string `json:"destination,omitempty"`
	// RecycleBin reports that a delete went to the Recycle Bin rather than
	// being unlinked.
	RecycleBin bool `json:"recycleBin"`
	// ArchiveID is the id that was invalidated.
	ArchiveID int64 `json:"archiveId"`
}

// releaseArchive closes and forgets an open archive handle so the file is no
// longer held by this process. Windows refuses to move or delete a file that
// still has an open handle, so this must happen before any filesystem change.
func (a *App) releaseArchive(id int64) {
	a.mu.Lock()
	delete(a.archives, id)
	a.mu.Unlock()
}

// releaseArchiveByPath forgets every handle pointing at path.
func (a *App) releaseArchiveByPath(path string) int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	var id int64
	for k, oa := range a.archives {
		if strings.EqualFold(oa.path, path) {
			id = k
			delete(a.archives, k)
		}
	}
	return id
}

// validateArchivePath checks that path is an existing ZIP/CBZ file.
func validateArchivePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if !isArchiveExt(abs) {
		return "", ErrNotArchive
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if fi.IsDir() {
		return "", ErrNotArchive
	}
	return abs, nil
}

func isArchiveExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".zip", ".cbz":
		return true
	}
	return false
}

// MoveArchive moves an archive to a destination directory. It refuses to
// overwrite an existing file, refuses to move a file onto itself, and requires
// the destination to stay inside the scanned root so a stray call cannot
// scatter files across the disk.
func (a *App) MoveArchive(id int64, destDir string) (OrganizeResult, error) {
	oa, err := a.lookup(id)
	if err != nil {
		return OrganizeResult{}, err
	}
	src, err := validateArchivePath(oa.path)
	if err != nil {
		return OrganizeResult{}, err
	}

	destDir, err = filepath.Abs(destDir)
	if err != nil {
		return OrganizeResult{}, err
	}
	fi, err := os.Stat(destDir)
	if err != nil {
		return OrganizeResult{}, fmt.Errorf("app: destination directory: %w", err)
	}
	if !fi.IsDir() {
		return OrganizeResult{}, fmt.Errorf("app: destination is not a directory")
	}

	root := a.cfg.Get().Root
	if root != "" {
		rootAbs, err := filepath.Abs(root)
		if err == nil && !isWithin(rootAbs, destDir) {
			return OrganizeResult{}, ErrOutsideRoot
		}
	}

	dest := filepath.Join(destDir, filepath.Base(src))
	if strings.EqualFold(dest, src) {
		return OrganizeResult{}, ErrSamePath
	}
	// Never overwrite: the user must resolve the conflict themselves.
	if _, err := os.Stat(dest); err == nil {
		return OrganizeResult{}, ErrTargetExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return OrganizeResult{}, err
	}

	// Release the handle before touching the file, otherwise Windows keeps the
	// file locked.
	a.releaseArchive(id)

	if err := os.Rename(src, dest); err != nil {
		// A cross-volume move cannot be a rename; fall back to copy+delete.
		if err := copyThenRemove(src, dest); err != nil {
			return OrganizeResult{}, err
		}
	}

	// The old path is gone, so its cache rows are meaningless.
	if err := a.st.DeleteArchive(src); err != nil {
		return OrganizeResult{}, err
	}
	return OrganizeResult{Source: src, Destination: dest, ArchiveID: id}, nil
}

// DeleteArchiveToRecycleBin moves an archive to the Recycle Bin. The file is
// recoverable, which is why this is the default delete action.
func (a *App) DeleteArchiveToRecycleBin(id int64) (OrganizeResult, error) {
	oa, err := a.lookup(id)
	if err != nil {
		return OrganizeResult{}, err
	}
	src, err := validateArchivePath(oa.path)
	if err != nil {
		return OrganizeResult{}, err
	}

	a.releaseArchive(id)

	if err := recycleFile(src); err != nil {
		return OrganizeResult{}, err
	}
	if err := a.st.DeleteArchive(src); err != nil {
		return OrganizeResult{}, err
	}
	return OrganizeResult{Source: src, RecycleBin: true, ArchiveID: id}, nil
}

// RenameArchive renames an archive in place. The extension is preserved so the
// file stays recognisable as a ZIP/CBZ.
func (a *App) RenameArchive(id int64, newName string) (OrganizeResult, error) {
	oa, err := a.lookup(id)
	if err != nil {
		return OrganizeResult{}, err
	}
	src, err := validateArchivePath(oa.path)
	if err != nil {
		return OrganizeResult{}, err
	}

	newName = strings.TrimSpace(newName)
	if newName == "" {
		return OrganizeResult{}, fmt.Errorf("app: empty name")
	}
	// Reject anything that could escape the directory.
	if strings.ContainsAny(newName, `/\:*?"<>|`) || newName == "." || newName == ".." {
		return OrganizeResult{}, fmt.Errorf("app: invalid file name")
	}
	if !isArchiveExt(newName) {
		newName += filepath.Ext(src)
	}

	dest := filepath.Join(filepath.Dir(src), newName)
	if strings.EqualFold(dest, src) {
		return OrganizeResult{}, ErrSamePath
	}
	if _, err := os.Stat(dest); err == nil {
		return OrganizeResult{}, ErrTargetExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return OrganizeResult{}, err
	}

	a.releaseArchive(id)
	if err := os.Rename(src, dest); err != nil {
		return OrganizeResult{}, err
	}
	if err := a.st.DeleteArchive(src); err != nil {
		return OrganizeResult{}, err
	}
	return OrganizeResult{Source: src, Destination: dest, ArchiveID: id}, nil
}

// isWithin reports whether path is inside root (or equal to it).
func isWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

// copyThenRemove copies src to dest and removes src, used when a rename crosses
// a volume boundary. The copy is written to a temporary file first so a failure
// never leaves a truncated archive behind.
func copyThenRemove(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".move-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.ReadFrom(in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return err
	}
	return os.Remove(src)
}

// OrganizeCapabilities reports what the current build supports, so the UI can
// hide actions that would fail.
type OrganizeCapabilities struct {
	Move        bool `json:"move"`
	Rename      bool `json:"rename"`
	RecycleBin  bool `json:"recycleBin"`
	PermanentOK bool `json:"permanentDelete"`
}

// GetOrganizeCapabilities returns the supported organize actions.
func (a *App) GetOrganizeCapabilities() OrganizeCapabilities {
	return OrganizeCapabilities{
		Move:        true,
		Rename:      true,
		RecycleBin:  recycleSupported(),
		PermanentOK: false,
	}
}

// RefreshArchive re-indexes an archive after an external change.
func (a *App) RefreshArchive(path string) (ArchiveSummary, error) {
	a.releaseArchiveByPath(path)
	return a.IndexArchive(path)
}

// PruneMissingArchives removes cache rows for archives that no longer exist.
func (a *App) PruneMissingArchives() (int, error) {
	rows, err := a.st.AllArchivePaths()
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, p := range rows {
		if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
			if err := a.st.DeleteArchive(p); err == nil {
				removed++
			}
		}
	}
	return removed, nil
}

// touchArchive records that an archive was used, for diagnostics.
func (a *App) touchArchive(id int64) {
	a.mu.Lock()
	if oa, ok := a.archives[id]; ok {
		oa.lastUse = time.Now()
	}
	a.mu.Unlock()
}

var _ = store.StatusOK
