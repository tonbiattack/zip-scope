package archive

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestReadEntryDetectsReplacedArchive verifies that an archive replaced after
// indexing is not read through the old ordinals.
func TestReadEntryDetectsReplacedArchive(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "swap.zip")
	writeZip(t, p, []zipEntry{
		{"a.jpg", makeJPEG(t, 8, 8)},
		{"b.jpg", makeJPEG(t, 16, 16)},
	})

	a, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	first := a.Entries()[0]

	// Replace the archive with different entries of the same count.
	writeZip(t, p, []zipEntry{
		{"x.jpg", makeJPEG(t, 32, 32)},
		{"y.jpg", makeJPEG(t, 48, 48)},
	})

	_, err = a.ReadEntry(first.Ordinal)
	if err == nil {
		t.Fatal("expected an error reading a replaced archive")
	}
	if !isErr(err, ErrArchiveChanged) {
		t.Fatalf("err = %v, want ErrArchiveChanged", err)
	}
}

// TestChangedDetectsReplacementWithRestoredMtime covers the case the issue
// calls out: a replacement that keeps the modification time, so only the
// central directory fingerprint reveals it.
func TestChangedDetectsReplacementWithRestoredMtime(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "same.zip")

	writeZip(t, p, []zipEntry{{"a.jpg", makeJPEG(t, 16, 16)}})
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	a, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// Replace the entry with different content, then restore the timestamp so
	// the mtime check alone would miss it.
	writeZip(t, p, []zipEntry{{"a.jpg", makeJPEG(t, 40, 40)}})
	if err := os.Chtimes(p, info.ModTime(), info.ModTime()); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if !a.Changed() {
		t.Error("Changed() did not detect a replacement with a restored mtime")
	}
	// The read must refuse the stale ordinal.
	if _, err := a.ReadEntry(a.Entries()[0].Ordinal); !isErr(err, ErrArchiveChanged) {
		t.Errorf("err = %v, want ErrArchiveChanged", err)
	}
}

// TestChangedReportsMissingFile verifies that a deleted archive counts as a
// change, which is what an unplugged drive looks like.
func TestChangedReportsMissingFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gone.zip")
	writeZip(t, p, []zipEntry{{"a.jpg", makeJPEG(t, 8, 8)}})

	a, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if a.Changed() {
		t.Error("Changed() reported a change before anything happened")
	}

	if err := os.Remove(p); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !a.Changed() {
		t.Error("Changed() did not report a missing file")
	}
}

// TestChangedIsFalseForUnchangedArchive verifies the common case stays cheap
// and correct.
func TestChangedIsFalseForUnchangedArchive(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "stable.zip")
	writeZip(t, p, []zipEntry{{"a.jpg", makeJPEG(t, 8, 8)}})

	a, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if a.Changed() {
		t.Error("Changed() reported a change for an untouched archive")
	}
	// Reading still works.
	if _, err := a.ReadEntry(a.Entries()[0].Ordinal); err != nil {
		t.Errorf("read: %v", err)
	}
}

// TestReadEntryDetectsTruncatedArchive verifies that a partially written file
// is reported as a change rather than read as garbage.
func TestReadEntryDetectsTruncatedArchive(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "trunc.zip")
	writeZip(t, p, []zipEntry{{"a.jpg", makeJPEG(t, 64, 64)}})

	a, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ord := a.Entries()[0].Ordinal

	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// Cut the file in half, destroying the central directory.
	if err := os.WriteFile(p, data[:len(data)/2], 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err = a.ReadEntry(ord)
	if err == nil {
		t.Fatal("expected an error reading a truncated archive")
	}
	// Either the archive is reported as changed or as corrupt; both are honest
	// answers and neither returns the wrong image.
	if !isErr(err, ErrArchiveChanged) && !isErr(err, ErrCorrupt) {
		t.Errorf("err = %v, want ErrArchiveChanged or ErrCorrupt", err)
	}
}

// TestReadEntryDetectsCorruptReplacement verifies that replacing a good archive
// with garbage is reported, not silently misread.
func TestReadEntryDetectsCorruptReplacement(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "corrupt.zip")
	writeZip(t, p, []zipEntry{{"a.jpg", makeJPEG(t, 8, 8)}})

	a, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ord := a.Entries()[0].Ordinal

	if err := os.WriteFile(p, []byte("not a zip file at all"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := a.ReadEntry(ord); err == nil {
		t.Fatal("expected an error reading a corrupt replacement")
	}
}

// TestReadEntryDetectsRenamedEntry verifies that an entry whose name changed at
// the same ordinal is not returned.
func TestReadEntryDetectsRenamedEntry(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "rename.zip")
	writeZip(t, p, []zipEntry{{"a.jpg", makeJPEG(t, 8, 8)}})

	a, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ord := a.Entries()[0].Ordinal

	// Same count, same position, different name.
	writeZip(t, p, []zipEntry{{"different.jpg", makeJPEG(t, 8, 8)}})

	_, err = a.ReadEntry(ord)
	if !isErr(err, ErrArchiveChanged) {
		t.Fatalf("err = %v, want ErrArchiveChanged", err)
	}
}

// TestReadEntryDetectsReorderedEntries verifies that a reordering at the same
// ordinal is caught by the CRC comparison.
func TestReadEntryDetectsReorderedEntries(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "reorder.zip")
	imgA := makeJPEG(t, 8, 8)
	imgB := makeJPEG(t, 24, 24)
	writeZip(t, p, []zipEntry{{"a.jpg", imgA}, {"b.jpg", imgB}})

	a, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// The natural order puts a.jpg first; remember its ordinal.
	first := a.Entries()[0]

	// Swap the entries so the same ordinal now holds the other image.
	writeZip(t, p, []zipEntry{{"b.jpg", imgB}, {"a.jpg", imgA}})

	_, err = a.ReadEntry(first.Ordinal)
	if !isErr(err, ErrArchiveChanged) {
		t.Fatalf("err = %v, want ErrArchiveChanged", err)
	}
}

// TestOpenStillReadsAfterReopen verifies that a fresh Open sees the new
// contents, which is what the app does after detecting a change.
func TestOpenStillReadsAfterReopen(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "reopen.zip")
	writeZip(t, p, []zipEntry{{"a.jpg", makeJPEG(t, 8, 8)}})

	a1, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	writeZip(t, p, []zipEntry{{"a.jpg", makeJPEG(t, 8, 8)}, {"b.jpg", makeJPEG(t, 8, 8)}})

	a2, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if a2.Count() != 2 {
		t.Errorf("count = %d, want 2", a2.Count())
	}
	if a1.Fingerprint() == a2.Fingerprint() {
		t.Error("the fingerprint did not change")
	}
	// The fresh handle reads without error.
	if _, err := a2.ReadEntry(a2.Entries()[0].Ordinal); err != nil {
		t.Errorf("read after reopen: %v", err)
	}
}

// TestChangedDetectsTouchOnly verifies that a bare mtime change is reported,
// because a tool may rewrite the file and restore the timestamp.
func TestChangedDetectsTouchOnly(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "touch.zip")
	writeZip(t, p, []zipEntry{{"a.jpg", makeJPEG(t, 8, 8)}})

	a, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(p, later, later); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if !a.Changed() {
		t.Error("Changed() did not notice a modified timestamp")
	}
}
