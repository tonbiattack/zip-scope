package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// setupOrganize creates an app with a scanned root containing one archive.
func setupOrganize(t *testing.T) (*App, string, ArchiveSummary) {
	t.Helper()
	a := testApp(t)
	root := t.TempDir()
	p := filepath.Join(root, "book.zip")
	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 100, 100, 10)}})

	// Record the root so the destination check has something to compare with.
	rootCopy := root
	if _, err := a.UpdateSettings(SettingsPatch{Root: &rootCopy}); err != nil {
		t.Fatalf("set root: %v", err)
	}
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	return a, root, sum
}

func TestMoveArchive(t *testing.T) {
	a, root, sum := setupOrganize(t)
	destDir := filepath.Join(root, "sorted")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	res, err := a.MoveArchive(sum.ID, destDir)
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if res.Destination != filepath.Join(destDir, "book.zip") {
		t.Errorf("destination = %q", res.Destination)
	}
	if _, err := os.Stat(res.Source); !os.IsNotExist(err) {
		t.Error("source still exists after the move")
	}
	if _, err := os.Stat(res.Destination); err != nil {
		t.Errorf("destination missing: %v", err)
	}
	// The cache row for the old path must be gone.
	row, err := a.st.GetArchive(res.Source)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row != nil {
		t.Error("cache row for the old path survived the move")
	}
}

func TestMoveArchiveRefusesOverwrite(t *testing.T) {
	a, root, sum := setupOrganize(t)
	destDir := filepath.Join(root, "sorted")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// A file already occupies the destination name.
	existing := filepath.Join(destDir, "book.zip")
	if err := os.WriteFile(existing, []byte("do not clobber"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := a.MoveArchive(sum.ID, destDir); !errors.Is(err, ErrTargetExists) {
		t.Fatalf("err = %v, want ErrTargetExists", err)
	}
	// The existing file must be untouched.
	data, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "do not clobber" {
		t.Error("the existing destination file was overwritten")
	}
	// The source must still be there.
	if _, err := os.Stat(filepath.Join(root, "book.zip")); err != nil {
		t.Error("source was removed despite the failure")
	}
}

func TestMoveArchiveRefusesSamePath(t *testing.T) {
	a, root, sum := setupOrganize(t)
	if _, err := a.MoveArchive(sum.ID, root); !errors.Is(err, ErrSamePath) {
		t.Fatalf("err = %v, want ErrSamePath", err)
	}
}

func TestMoveArchiveRefusesOutsideRoot(t *testing.T) {
	a, _, sum := setupOrganize(t)
	outside := t.TempDir()
	if _, err := a.MoveArchive(sum.ID, outside); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("err = %v, want ErrOutsideRoot", err)
	}
}

func TestMoveArchiveRefusesMissingDestination(t *testing.T) {
	a, root, sum := setupOrganize(t)
	if _, err := a.MoveArchive(sum.ID, filepath.Join(root, "nope")); err == nil {
		t.Fatal("expected an error for a missing destination directory")
	}
}

func TestMoveArchiveReleasesHandle(t *testing.T) {
	a, root, sum := setupOrganize(t)
	// Open the archive so a handle is definitely held.
	if _, err := a.Thumbnail(sum.ID, sum.CoverOrdinal, 192); err != nil {
		t.Fatalf("thumb: %v", err)
	}
	destDir := filepath.Join(root, "moved")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// On Windows this fails with a sharing violation if the handle is still
	// open, so a successful move proves the release happened.
	if _, err := a.MoveArchive(sum.ID, destDir); err != nil {
		t.Fatalf("move with an open handle: %v", err)
	}
}

func TestRenameArchive(t *testing.T) {
	a, root, sum := setupOrganize(t)
	res, err := a.RenameArchive(sum.ID, "renamed")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	// The extension is preserved when the new name omits it.
	if filepath.Base(res.Destination) != "renamed.zip" {
		t.Errorf("destination = %q, want renamed.zip", res.Destination)
	}
	if _, err := os.Stat(filepath.Join(root, "renamed.zip")); err != nil {
		t.Errorf("renamed file missing: %v", err)
	}
}

func TestRenameArchiveRejectsPathTraversal(t *testing.T) {
	a, _, sum := setupOrganize(t)
	bad := []string{"..", ".", `..\escape`, "sub/dir", `C:\evil`, "a:b", "a*b", "a?b", "a|b", `a"b`, "a<b", "a>b"}
	for _, name := range bad {
		if _, err := a.RenameArchive(sum.ID, name); err == nil {
			t.Errorf("RenameArchive(%q) succeeded, want an error", name)
		}
	}
}

func TestRenameArchiveRefusesOverwrite(t *testing.T) {
	a, root, sum := setupOrganize(t)
	other := filepath.Join(root, "other.zip")
	if err := os.WriteFile(other, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := a.RenameArchive(sum.ID, "other.zip"); !errors.Is(err, ErrTargetExists) {
		t.Fatalf("err = %v, want ErrTargetExists", err)
	}
	data, err := os.ReadFile(other)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "keep me" {
		t.Error("the existing file was overwritten")
	}
}

func TestRenameArchiveRejectsNonArchive(t *testing.T) {
	a := testApp(t)
	root := t.TempDir()
	p := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Indexing a non-archive fails, so there is no id to rename; the guard is
	// exercised through validateArchivePath instead.
	if _, err := validateArchivePath(p); !errors.Is(err, ErrNotArchive) {
		t.Errorf("err = %v, want ErrNotArchive", err)
	}
	_ = a
}

func TestDeleteArchiveToRecycleBin(t *testing.T) {
	a, root, sum := setupOrganize(t)
	src := filepath.Join(root, "book.zip")

	res, err := a.DeleteArchiveToRecycleBin(sum.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !res.RecycleBin {
		t.Error("result did not report the Recycle Bin")
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("file still exists after the delete")
	}
	row, err := a.st.GetArchive(src)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row != nil {
		t.Error("cache row survived the delete")
	}
}

func TestDeleteReleasesHandle(t *testing.T) {
	a, _, sum := setupOrganize(t)
	// Hold a handle, then delete: Windows would refuse without the release.
	if _, err := a.Thumbnail(sum.ID, sum.CoverOrdinal, 192); err != nil {
		t.Fatalf("thumb: %v", err)
	}
	if _, err := a.DeleteArchiveToRecycleBin(sum.ID); err != nil {
		t.Fatalf("delete with an open handle: %v", err)
	}
}

func TestOrganizeCapabilities(t *testing.T) {
	a := testApp(t)
	caps := a.GetOrganizeCapabilities()
	if !caps.Move || !caps.Rename {
		t.Errorf("caps = %+v", caps)
	}
	if !caps.RecycleBin {
		t.Error("Recycle Bin should be supported on Windows")
	}
	if caps.PermanentOK {
		t.Error("permanent delete must not be offered")
	}
}

func TestIsWithin(t *testing.T) {
	root := `C:\pics`
	cases := []struct {
		path string
		want bool
	}{
		{`C:\pics`, true},
		{`C:\pics\sub`, true},
		{`C:\pics\sub\deep`, true},
		{`C:\pics2`, false},
		{`C:\other`, false},
		{`C:\`, false},
	}
	for _, c := range cases {
		if got := isWithin(root, c.path); got != c.want {
			t.Errorf("isWithin(%q, %q) = %v, want %v", root, c.path, got, c.want)
		}
	}
}

func TestPruneMissingArchives(t *testing.T) {
	a, root, sum := setupOrganize(t)
	// A second archive that will disappear.
	gone := filepath.Join(root, "gone.zip")
	makeZip(t, gone, []entry{{"b.jpg", encodeJPEG(t, 50, 50, 20)}})
	if _, err := a.IndexArchive(gone); err != nil {
		t.Fatalf("index: %v", err)
	}
	if err := os.Remove(gone); err != nil {
		t.Fatalf("remove: %v", err)
	}

	removed, err := a.PruneMissingArchives()
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	// The surviving archive must still be cached.
	row, err := a.st.GetArchive(filepath.Join(root, "book.zip"))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row == nil {
		t.Error("a live archive was pruned")
	}
	_ = sum
}

func TestRefreshArchive(t *testing.T) {
	a, root, sum := setupOrganize(t)
	p := filepath.Join(root, "book.zip")
	// Replace the archive with more images.
	makeZip(t, p, []entry{
		{"a.jpg", encodeJPEG(t, 100, 100, 10)},
		{"b.jpg", encodeJPEG(t, 100, 100, 20)},
	})
	refreshed, err := a.RefreshArchive(p)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if refreshed.Count != 2 {
		t.Errorf("count = %d, want 2", refreshed.Count)
	}
	if refreshed.ID != sum.ID {
		t.Errorf("id changed: %d -> %d", sum.ID, refreshed.ID)
	}
}
