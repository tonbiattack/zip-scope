package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/tonbiattack/zip-scope/internal/archive"
)

// TestThumbnailSurvivesArchiveReplacement verifies that replacing an archive
// while it is open does not serve the old image under the old ordinal.
func TestThumbnailSurvivesArchiveReplacement(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "swap.zip")

	// Two visually distinct images so a mix-up is detectable.
	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 200, 200, 10)}})
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	before, err := a.Thumbnail(sum.ID, sum.CoverOrdinal, 192)
	if err != nil {
		t.Fatalf("thumb: %v", err)
	}

	// Replace the archive with a different image at the same ordinal.
	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 200, 200, 240)}})

	after, err := a.Thumbnail(sum.ID, sum.CoverOrdinal, 192)
	if err != nil {
		t.Fatalf("thumb after replacement: %v", err)
	}
	if bytes.Equal(before.Data, after.Data) {
		t.Error("the old thumbnail was served after the archive was replaced")
	}
}

// TestFullImageSurvivesArchiveReplacement verifies the same for the full-size
// path, which is what the viewer displays.
func TestFullImageSurvivesArchiveReplacement(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "swap.zip")

	original := encodeJPEG(t, 120, 90, 30)
	makeZip(t, p, []entry{{"a.jpg", original}})
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	before, err := a.FullImage(sum.ID, sum.CoverOrdinal)
	if err != nil {
		t.Fatalf("full: %v", err)
	}
	if !bytes.Equal(before.Data, original) {
		t.Fatal("the first read did not return the original bytes")
	}

	replacement := encodeJPEG(t, 120, 90, 220)
	makeZip(t, p, []entry{{"a.jpg", replacement}})

	after, err := a.FullImage(sum.ID, sum.CoverOrdinal)
	if err != nil {
		t.Fatalf("full after replacement: %v", err)
	}
	if bytes.Equal(after.Data, original) {
		t.Error("the old image was served after the archive was replaced")
	}
	if !bytes.Equal(after.Data, replacement) {
		t.Error("the replacement image was not served")
	}
}

// TestReplacementWithFewerEntriesIsHandled verifies that an ordinal which no
// longer exists is reported rather than read out of range.
func TestReplacementWithFewerEntriesIsHandled(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "shrink.zip")

	makeZip(t, p, []entry{
		{"a.jpg", encodeJPEG(t, 100, 100, 10)},
		{"b.jpg", encodeJPEG(t, 100, 100, 60)},
		{"c.jpg", encodeJPEG(t, 100, 100, 120)},
	})
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	detail, err := a.GetArchiveEntries(sum.ID)
	if err != nil {
		t.Fatalf("entries: %v", err)
	}
	lastOrdinal := detail.Entries[len(detail.Entries)-1].Ordinal

	// Replace with a single entry, so the last ordinal no longer exists.
	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 100, 100, 200)}})

	_, err = a.FullImage(sum.ID, lastOrdinal)
	if err == nil {
		t.Fatal("expected an error for an ordinal that no longer exists")
	}
	ie := classifyImageError(err)
	if ie.Kind != ErrKindChanged && ie.Kind != ErrKindMissing {
		t.Errorf("kind = %q, want changed or missing", ie.Kind)
	}
}

// TestReplacementWithCorruptArchiveIsReported verifies that a good archive
// replaced by garbage yields an error, not a wrong image.
func TestReplacementWithCorruptArchiveIsReported(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "corrupt.zip")

	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 100, 100, 10)}})
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}

	if err := os.WriteFile(p, []byte("garbage"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := a.FullImage(sum.ID, sum.CoverOrdinal); err == nil {
		t.Fatal("expected an error for a corrupt replacement")
	}
}

// TestDeletedArchiveIsReported verifies that an unplugged drive is reported as
// a change rather than a crash.
func TestDeletedArchiveIsReported(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "gone.zip")

	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 100, 100, 10)}})
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}

	if err := os.Remove(p); err != nil {
		t.Fatalf("remove: %v", err)
	}

	_, err = a.FullImage(sum.ID, sum.CoverOrdinal)
	if err == nil {
		t.Fatal("expected an error for a deleted archive")
	}
}

// TestUnchangedArchiveIsNotReindexed verifies that the common case does not pay
// for a re-index on every read.
func TestUnchangedArchiveIsNotReindexed(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "stable.zip")
	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 100, 100, 10)}})

	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}

	oa, reindexed, err := a.reindexIfChanged(sum.ID)
	if err != nil {
		t.Fatalf("reindexIfChanged: %v", err)
	}
	if reindexed {
		t.Error("an untouched archive was re-indexed")
	}
	if oa == nil || oa.arc == nil {
		t.Fatal("no handle returned")
	}
}

// TestReindexPreservesArchiveID verifies that re-indexing keeps the id, so the
// UI selection survives a change.
func TestReindexPreservesArchiveID(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "keep.zip")
	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 100, 100, 10)}})

	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}

	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 100, 100, 200)}})

	oa, reindexed, err := a.reindexIfChanged(sum.ID)
	if err != nil {
		t.Fatalf("reindexIfChanged: %v", err)
	}
	if !reindexed {
		t.Error("the change was not detected")
	}
	if oa.id != sum.ID {
		t.Errorf("id changed: %d -> %d", sum.ID, oa.id)
	}
	// The fresh index reflects the new content.
	if _, err := oa.arc.ReadEntry(oa.arc.Entries()[0].Ordinal); err != nil {
		t.Errorf("read after re-index: %v", err)
	}
}

// TestChangedKindIsClassified verifies that the UI can tell a change apart from
// other failures.
func TestChangedKindIsClassified(t *testing.T) {
	ie := classifyImageError(archive.ErrArchiveChanged)
	if ie.Kind != ErrKindChanged {
		t.Errorf("kind = %q, want %q", ie.Kind, ErrKindChanged)
	}
}
