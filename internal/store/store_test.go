package store

import (
	"os"
	"path/filepath"
	"testing"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPutAndGetArchive(t *testing.T) {
	s := newStore(t)
	row := ArchiveRow{Path: `C:\a\b.zip`, Size: 100, MTime: 42, Fingerprint: "fp1", Status: StatusOK, Count: 2}
	entries := []EntryRow{
		{Ordinal: 0, Name: "a.jpg", CRC32: 1, CompressedSize: 10, UncompressedSize: 20},
		{Ordinal: 1, Name: "b.jpg", CRC32: 2, CompressedSize: 11, UncompressedSize: 21},
	}
	if err := s.PutArchive(row, entries); err != nil {
		t.Fatalf("put: %v", err)
	}

	got, err := s.GetArchive(row.Path)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("archive not found")
	}
	if got.Fingerprint != "fp1" || got.Count != 2 || got.Status != StatusOK {
		t.Errorf("row = %+v", got)
	}

	rows, err := s.Entries(got.ID)
	if err != nil {
		t.Fatalf("entries: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("entries = %d, want 2", len(rows))
	}
	if rows[0].Name != "a.jpg" || rows[1].Name != "b.jpg" {
		t.Errorf("entries = %+v", rows)
	}

	byID, err := s.GetArchiveByID(got.ID)
	if err != nil || byID == nil {
		t.Fatalf("get by id: %v %v", byID, err)
	}
	if byID.Path != row.Path {
		t.Errorf("path = %q", byID.Path)
	}
}

func TestPutArchiveReplacesEntries(t *testing.T) {
	s := newStore(t)
	row := ArchiveRow{Path: "x.zip", Size: 1, MTime: 1, Fingerprint: "a", Status: StatusOK, Count: 2}
	if err := s.PutArchive(row, []EntryRow{
		{Ordinal: 0, Name: "a.jpg"},
		{Ordinal: 1, Name: "b.jpg"},
	}); err != nil {
		t.Fatalf("put: %v", err)
	}
	// Re-indexing with fewer entries must not leave stale rows behind.
	row.Fingerprint = "b"
	row.Count = 1
	if err := s.PutArchive(row, []EntryRow{{Ordinal: 0, Name: "a.jpg"}}); err != nil {
		t.Fatalf("put2: %v", err)
	}
	got, err := s.GetArchive("x.zip")
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	rows, err := s.Entries(got.ID)
	if err != nil {
		t.Fatalf("entries: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("entries = %d, want 1", len(rows))
	}
	if got.Fingerprint != "b" {
		t.Errorf("fingerprint = %q, want b", got.Fingerprint)
	}
}

func TestThumbRoundTrip(t *testing.T) {
	s := newStore(t)
	data := []byte("fake-jpeg-bytes")
	path, err := s.PutThumb("key1", "ab/cd", ".jpg", data)
	if err != nil {
		t.Fatalf("put thumb: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("thumb file missing: %v", err)
	}
	got, ok := s.LookupThumb("key1")
	if !ok {
		t.Fatal("thumb not found")
	}
	if string(got) != string(data) {
		t.Errorf("thumb = %q, want %q", got, data)
	}
}

func TestThumbMissingFileIsAMiss(t *testing.T) {
	s := newStore(t)
	path, err := s.PutThumb("key2", "aa/bb", ".jpg", []byte("x"))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	// Simulate an external deletion: the row survives but the file does not.
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, ok := s.LookupThumb("key2"); ok {
		t.Fatal("expected a cache miss after the file was deleted")
	}
	// The stale row must be dropped so the next write is clean.
	count, _, err := s.ThumbStats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if count != 0 {
		t.Errorf("thumb rows = %d, want 0", count)
	}
}

func TestThumbAtomicWriteLeavesNoTempFiles(t *testing.T) {
	s := newStore(t)
	if _, err := s.PutThumb("k", "aa/bb", ".jpg", []byte("payload")); err != nil {
		t.Fatalf("put: %v", err)
	}
	dir := filepath.Join(s.ThumbDir(), "aa", "bb")
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

func TestEvictLRU(t *testing.T) {
	s := newStore(t)
	// A tiny budget forces eviction after a couple of writes.
	s.SetLimit(30)
	for i, key := range []string{"a", "b", "c", "d"} {
		if _, err := s.PutThumb(key, "aa/bb", ".jpg", make([]byte, 10)); err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
	}
	removed, err := s.Evict(true)
	if err != nil {
		t.Fatalf("evict: %v", err)
	}
	if removed == 0 {
		t.Fatal("nothing was evicted")
	}
	_, total, err := s.ThumbStats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if total > 30 {
		t.Errorf("total = %d, want <= 30", total)
	}
	// The most recently written entry must survive.
	if _, ok := s.LookupThumb("d"); !ok {
		t.Error("most recently used thumbnail was evicted")
	}
}

func TestClearThumbs(t *testing.T) {
	s := newStore(t)
	for _, k := range []string{"a", "b"} {
		if _, err := s.PutThumb(k, "aa/bb", ".jpg", []byte("x")); err != nil {
			t.Fatalf("put: %v", err)
		}
	}
	if err := s.ClearThumbs(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	count, total, err := s.ThumbStats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if count != 0 || total != 0 {
		t.Errorf("count = %d, total = %d, want 0/0", count, total)
	}
}

func TestClearAll(t *testing.T) {
	s := newStore(t)
	if err := s.PutArchive(ArchiveRow{Path: "a.zip", Size: 1, MTime: 1, Status: StatusOK}, nil); err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, err := s.PutThumb("k", "aa/bb", ".jpg", []byte("x")); err != nil {
		t.Fatalf("put thumb: %v", err)
	}
	if err := s.ClearAll(); err != nil {
		t.Fatalf("clear all: %v", err)
	}
	n, err := s.CountArchives()
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("archives = %d, want 0", n)
	}
}

func TestSweepOrphans(t *testing.T) {
	s := newStore(t)
	// A file with no row is an orphan from a crash between rename and insert.
	dir := filepath.Join(s.ThumbDir(), "zz", "yy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	orphan := filepath.Join(dir, "orphan.jpg")
	if err := os.WriteFile(orphan, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	removed, err := s.SweepOrphans()
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Error("orphan file still exists")
	}
}

func TestReopenPersists(t *testing.T) {
	dir := t.TempDir()
	s1, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s1.PutArchive(ArchiveRow{Path: "p.zip", Size: 5, MTime: 7, Fingerprint: "fp", Status: StatusOK, Count: 1},
		[]EntryRow{{Ordinal: 0, Name: "a.jpg"}}); err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, err := s1.PutThumb("k", "aa/bb", ".jpg", []byte("bytes")); err != nil {
		t.Fatalf("put thumb: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	row, err := s2.GetArchive("p.zip")
	if err != nil || row == nil {
		t.Fatalf("get after reopen: %v %v", row, err)
	}
	if row.Fingerprint != "fp" {
		t.Errorf("fingerprint = %q", row.Fingerprint)
	}
	if _, ok := s2.LookupThumb("k"); !ok {
		t.Error("thumbnail did not survive a restart")
	}
}

func TestDeleteArchiveCascadesEntries(t *testing.T) {
	s := newStore(t)
	if err := s.PutArchive(ArchiveRow{Path: "d.zip", Size: 1, MTime: 1, Status: StatusOK, Count: 1},
		[]EntryRow{{Ordinal: 0, Name: "a.jpg"}}); err != nil {
		t.Fatalf("put: %v", err)
	}
	row, _ := s.GetArchive("d.zip")
	if err := s.DeleteArchive("d.zip"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rows, err := s.Entries(row.ID)
	if err != nil {
		t.Fatalf("entries: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("entries = %d, want 0 after cascade", len(rows))
	}
}
