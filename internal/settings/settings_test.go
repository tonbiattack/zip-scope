package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	d := Default()
	if d.CacheLimitBytes != 2<<30 {
		t.Errorf("cache limit = %d, want 2 GiB", d.CacheLimitBytes)
	}
	if d.MemoryLimitBytes != 256<<20 {
		t.Errorf("memory limit = %d, want 256 MiB", d.MemoryLimitBytes)
	}
	if d.Workers != 4 {
		t.Errorf("workers = %d, want 4", d.Workers)
	}
	if d.MaxEntryBytes != 128<<20 {
		t.Errorf("entry limit = %d, want 128 MiB", d.MaxEntryBytes)
	}
	if d.MaxPixels != 40_000_000 {
		t.Errorf("pixel limit = %d, want 40 MP", d.MaxPixels)
	}
	if d.MaxEntries != 100_000 {
		t.Errorf("entry count limit = %d, want 100000", d.MaxEntries)
	}
}

func TestNormalizeClampsUnsafeValues(t *testing.T) {
	// A hand-edited file must not be able to disable a safety limit.
	s := Settings{
		ThumbSize:        -5,
		SortMode:         "bogus",
		CacheLimitBytes:  1,
		MemoryLimitBytes: 1,
		Workers:          0,
		PrefetchAhead:    -3,
		MaxEntryBytes:    0,
		MaxPixels:        0,
		MaxEntries:       0,
		WindowWidth:      10,
		WindowHeight:     10,
		SelectedEntry:    -1,
		ScrollTop:        -100,
	}
	s.Normalize()
	d := Default()
	if s.ThumbSize != d.ThumbSize {
		t.Errorf("thumb size = %d", s.ThumbSize)
	}
	if s.SortMode != d.SortMode {
		t.Errorf("sort mode = %q", s.SortMode)
	}
	if s.CacheLimitBytes != d.CacheLimitBytes {
		t.Errorf("cache limit = %d", s.CacheLimitBytes)
	}
	if s.MemoryLimitBytes != d.MemoryLimitBytes {
		t.Errorf("memory limit = %d", s.MemoryLimitBytes)
	}
	if s.Workers != d.Workers {
		t.Errorf("workers = %d", s.Workers)
	}
	if s.PrefetchAhead != d.PrefetchAhead {
		t.Errorf("prefetch ahead = %d", s.PrefetchAhead)
	}
	if s.MaxEntryBytes != d.MaxEntryBytes || s.MaxPixels != d.MaxPixels || s.MaxEntries != d.MaxEntries {
		t.Errorf("limits were not restored: %+v", s)
	}
	if s.WindowWidth != d.WindowWidth || s.WindowHeight != d.WindowHeight {
		t.Errorf("window size = %dx%d", s.WindowWidth, s.WindowHeight)
	}
	if s.SelectedEntry < 0 || s.ScrollTop < 0 {
		t.Errorf("negative position survived: %d %f", s.SelectedEntry, s.ScrollTop)
	}
}

func TestManagerPersists(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	got, err := m.Update(func(s *Settings) {
		s.Root = `C:\pics`
		s.ThumbSize = 256
		s.SortMode = "mtime"
		s.SortDesc = true
		s.SelectedEntry = 7
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Root != `C:\pics` || got.ThumbSize != 256 || !got.SortDesc {
		t.Errorf("settings = %+v", got)
	}

	// A fresh manager must read the same values back.
	m2, err := NewManager(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got2 := m2.Get()
	if got2.Root != got.Root || got2.ThumbSize != got.ThumbSize || got2.SortMode != got.SortMode {
		t.Errorf("reloaded = %+v, want %+v", got2, got)
	}
}

func TestManagerMissingFileUsesDefaults(t *testing.T) {
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if got := m.Get(); got.CacheLimitBytes != Default().CacheLimitBytes {
		t.Errorf("cache limit = %d", got.CacheLimitBytes)
	}
}

func TestManagerCorruptFileFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if got := m.Get(); got.Workers != Default().Workers {
		t.Errorf("workers = %d, want the default", got.Workers)
	}
}

func TestManagerPartialFileKeepsDefaults(t *testing.T) {
	dir := t.TempDir()
	// Only one field is present; the rest must come from the defaults.
	data, _ := json.Marshal(map[string]any{"thumbSize": 384})
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	got := m.Get()
	if got.ThumbSize != 384 {
		t.Errorf("thumb size = %d, want 384", got.ThumbSize)
	}
	if got.CacheLimitBytes != Default().CacheLimitBytes {
		t.Errorf("cache limit = %d, want the default", got.CacheLimitBytes)
	}
}

func TestManagerAtomicWriteLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if _, err := m.Update(func(s *Settings) { s.ThumbSize = 128 }); err != nil {
		t.Fatalf("update: %v", err)
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

func TestManagerConcurrentUpdates(t *testing.T) {
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			_, _ = m.Update(func(s *Settings) { s.SelectedEntry = i })
			_ = m.Get()
		}(i)
	}
	for i := 0; i < 20; i++ {
		<-done
	}
	// The file must still be valid JSON after concurrent writes.
	data, err := os.ReadFile(m.Path())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("settings file is corrupt: %v", err)
	}
}
