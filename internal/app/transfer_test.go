package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeFile creates a file with the given content.
func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestCopyVerifiedProducesIdenticalFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.zip")
	dest := filepath.Join(dir, "out", "dest.zip")
	data := []byte("archive bytes that must survive the copy")
	writeFile(t, src, data)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := copyVerified(src, dest); err != nil {
		t.Fatalf("copyVerified: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("dest = %q, want %q", got, data)
	}
	// The source is removed only after the copy is verified.
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("source still exists after a successful move")
	}
}

func TestCopyVerifiedRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.zip")
	dest := filepath.Join(dir, "dest.zip")
	writeFile(t, src, []byte("new content"))
	writeFile(t, dest, []byte("existing content that must not be lost"))

	err := copyVerified(src, dest)
	if !errors.Is(err, ErrTargetExists) {
		t.Fatalf("err = %v, want ErrTargetExists", err)
	}

	// The existing file is untouched and the source is still there.
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != "existing content that must not be lost" {
		t.Errorf("dest was overwritten: %q", got)
	}
	if _, err := os.Stat(src); err != nil {
		t.Error("source was removed despite the failure")
	}
}

func TestCopyVerifiedLeavesNoTempFileOnFailure(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.zip")
	dest := filepath.Join(dir, "dest.zip")
	writeFile(t, src, []byte("content"))
	writeFile(t, dest, []byte("occupied"))

	if err := copyVerified(src, dest); err == nil {
		t.Fatal("expected the copy to fail")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if len(e.Name()) > 0 && e.Name()[0] == '.' {
			t.Errorf("temporary file left behind: %s", e.Name())
		}
	}
}

func TestCopyVerifiedMissingSource(t *testing.T) {
	dir := t.TempDir()
	err := copyVerified(filepath.Join(dir, "gone.zip"), filepath.Join(dir, "dest.zip"))
	if err == nil {
		t.Fatal("expected an error for a missing source")
	}
	// Nothing is created when the source cannot be opened.
	if _, err := os.Stat(filepath.Join(dir, "dest.zip")); !os.IsNotExist(err) {
		t.Error("a destination was created for a missing source")
	}
}

func TestCopyVerifiedMissingDestinationDirectory(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.zip")
	writeFile(t, src, []byte("content"))

	err := copyVerified(src, filepath.Join(dir, "no-such-dir", "dest.zip"))
	if err == nil {
		t.Fatal("expected an error for a missing destination directory")
	}
	// The source must survive.
	if _, err := os.Stat(src); err != nil {
		t.Error("source was lost when the destination directory was missing")
	}
}

func TestFileDigestDetectsDifference(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.bin")
	b := filepath.Join(dir, "b.bin")
	writeFile(t, a, []byte("same length"))
	writeFile(t, b, []byte("diff length"))

	_, sumA, err := fileDigest(a)
	if err != nil {
		t.Fatalf("digest a: %v", err)
	}
	_, sumB, err := fileDigest(b)
	if err != nil {
		t.Fatalf("digest b: %v", err)
	}
	if sumA == sumB {
		t.Error("different content produced the same digest")
	}
}

func TestFileDigestStableForSameContent(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.bin")
	b := filepath.Join(dir, "b.bin")
	writeFile(t, a, []byte("identical"))
	writeFile(t, b, []byte("identical"))

	sizeA, sumA, err := fileDigest(a)
	if err != nil {
		t.Fatalf("digest a: %v", err)
	}
	sizeB, sumB, err := fileDigest(b)
	if err != nil {
		t.Fatalf("digest b: %v", err)
	}
	if sizeA != sizeB || sumA != sumB {
		t.Error("identical content produced different digests")
	}
}

func TestPublishNoReplaceRefusesExisting(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.tmp")
	dest := filepath.Join(dir, "dest.zip")
	writeFile(t, src, []byte("new"))
	writeFile(t, dest, []byte("old"))

	if err := publishNoReplace(src, dest); !errors.Is(err, ErrTargetExists) {
		t.Fatalf("err = %v, want ErrTargetExists", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != "old" {
		t.Errorf("dest was replaced: %q", got)
	}
}

func TestPublishNoReplaceMovesFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.tmp")
	dest := filepath.Join(dir, "dest.zip")
	writeFile(t, src, []byte("payload"))

	if err := publishNoReplace(src, dest); err != nil {
		t.Fatalf("publish: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("dest = %q", got)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("source still exists after publishing")
	}
}

func TestMoveFileSameVolume(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.zip")
	dest := filepath.Join(dir, "dest.zip")
	writeFile(t, src, []byte("content"))

	if err := moveFile(src, dest); err != nil {
		t.Fatalf("moveFile: %v", err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("destination missing: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("source still exists")
	}
}

func TestMoveFileMissingSourceIsNotRetriedAsCopy(t *testing.T) {
	dir := t.TempDir()
	err := moveFile(filepath.Join(dir, "gone.zip"), filepath.Join(dir, "dest.zip"))
	if err == nil {
		t.Fatal("expected an error")
	}
	// A missing source is not a cross-device failure, so it must not be
	// reported as a copy problem.
	if errors.Is(err, ErrCopyIncomplete) || errors.Is(err, ErrCopyMismatch) {
		t.Errorf("err = %v, want the rename error", err)
	}
}

func TestMoveArchiveRefusesOverwriteRace(t *testing.T) {
	a, root, sum := setupOrganize(t)
	destDir := filepath.Join(root, "sorted")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// A file appears at the destination after the caller's check would have
	// run. The move must still refuse to replace it.
	existing := filepath.Join(destDir, "book.zip")
	writeFile(t, existing, []byte("must not be replaced"))

	if _, err := a.MoveArchive(sum.ID, destDir); !errors.Is(err, ErrTargetExists) {
		t.Fatalf("err = %v, want ErrTargetExists", err)
	}
	got, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "must not be replaced" {
		t.Error("the destination was overwritten")
	}
	// The source survives.
	if _, err := os.Stat(filepath.Join(root, "book.zip")); err != nil {
		t.Error("source was removed despite the failure")
	}
}

func TestMoveArchiveRejectsSymlinkedDestination(t *testing.T) {
	a, root, sum := setupOrganize(t)
	outside := t.TempDir()

	// A link inside the root that points outside it. Following it would place
	// the file beyond the scanned root.
	link := filepath.Join(root, "escape")
	if err := makeDirLink(outside, link); err != nil {
		t.Skipf("cannot create a link: %v", err)
	}

	if _, err := a.MoveArchive(sum.ID, link); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("err = %v, want ErrOutsideRoot", err)
	}
	// Nothing was written outside the root.
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatalf("read outside: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("files were written outside the root: %v", entries)
	}
}

func TestMoveArchiveKeepsSourceWhenDestinationIsLocked(t *testing.T) {
	a, root, sum := setupOrganize(t)
	destDir := filepath.Join(root, "locked")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Hold the destination open so the publish cannot complete.
	dest := filepath.Join(destDir, "book.zip")
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()

	if _, err := a.MoveArchive(sum.ID, destDir); err == nil {
		t.Fatal("expected the move to fail")
	}
	// The original must still be readable.
	if _, err := os.Stat(filepath.Join(root, "book.zip")); err != nil {
		t.Errorf("source was lost: %v", err)
	}
}

func TestResolveRealPath(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(dir, "link")
	if err := makeDirLink(real, link); err != nil {
		t.Skipf("cannot create a link: %v", err)
	}

	got, err := resolveRealPath(link)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if got != want {
		t.Errorf("resolveRealPath = %q, want %q", got, want)
	}
}
