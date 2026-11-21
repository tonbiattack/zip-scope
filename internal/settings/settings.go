// Package settings persists user preferences as JSON next to the cache.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Settings is the persisted user state. Every field has a safe zero value so a
// missing or partially written file still yields a usable configuration.
type Settings struct {
	// Root is the last scanned folder.
	Root string `json:"root"`
	// SelectedArchive is the last selected archive path.
	SelectedArchive string `json:"selectedArchive"`
	// SelectedEntry is the last selected entry ordinal within that archive.
	SelectedEntry int `json:"selectedEntry"`
	// ScrollTop is the saved grid scroll offset in pixels.
	ScrollTop float64 `json:"scrollTop"`
	// ThumbSize is the grid thumbnail width in pixels.
	ThumbSize int `json:"thumbSize"`
	// ViewMode is "details", "medium" or "large".
	ViewMode string `json:"viewMode"`
	// IconSize is the icon width in pixels for the icon views.
	IconSize int `json:"iconSize"`
	// SortMode is one of "name", "mtime", "size".
	SortMode string `json:"sortMode"`
	// SortDesc reverses the sort.
	SortDesc bool `json:"sortDesc"`
	// Search is the last search query.
	Search string `json:"search"`
	// CacheLimitBytes is the thumbnail disk budget.
	CacheLimitBytes int64 `json:"cacheLimitBytes"`
	// MemoryLimitBytes is the decoded-image memory budget.
	MemoryLimitBytes int64 `json:"memoryLimitBytes"`
	// Workers is the prefetch worker count.
	Workers int `json:"workers"`
	// PrefetchAhead is how many neighbouring images to prefetch.
	PrefetchAhead int `json:"prefetchAhead"`
	// MaxEntryBytes caps one entry's uncompressed size.
	MaxEntryBytes int64 `json:"maxEntryBytes"`
	// MaxPixels caps one image's pixel count.
	MaxPixels int64 `json:"maxPixels"`
	// MaxEntries caps entries per archive.
	MaxEntries int `json:"maxEntries"`
	// WindowWidth and WindowHeight restore the window size.
	WindowWidth  int `json:"windowWidth"`
	WindowHeight int `json:"windowHeight"`
}

// Default returns the MVP defaults from the specification.
func Default() Settings {
	return Settings{
		ThumbSize:        192,
		ViewMode:         "details",
		IconSize:         128,
		SortMode:         "name",
		CacheLimitBytes:  2 << 30, // 2 GiB
		MemoryLimitBytes: 256 << 20,
		Workers:          4,
		PrefetchAhead:    2,
		MaxEntryBytes:    128 << 20,
		MaxPixels:        40_000_000,
		MaxEntries:       100_000,
		WindowWidth:      1440,
		WindowHeight:     900,
	}
}

// Normalize clamps values into supported ranges and fills in defaults for
// anything unset, so a hand-edited file cannot disable a safety limit.
func (s *Settings) Normalize() {
	d := Default()
	if s.ThumbSize < 96 || s.ThumbSize > 512 {
		s.ThumbSize = d.ThumbSize
	}
	switch s.ViewMode {
	case "details", "medium", "large":
	default:
		s.ViewMode = d.ViewMode
	}
	if s.IconSize < 64 || s.IconSize > 256 {
		s.IconSize = d.IconSize
	}
	switch s.SortMode {
	case "name", "mtime", "size":
	default:
		s.SortMode = d.SortMode
	}
	if s.CacheLimitBytes < 64<<20 {
		s.CacheLimitBytes = d.CacheLimitBytes
	}
	if s.MemoryLimitBytes < 32<<20 {
		s.MemoryLimitBytes = d.MemoryLimitBytes
	}
	if s.Workers < 1 || s.Workers > 16 {
		s.Workers = d.Workers
	}
	if s.PrefetchAhead < 0 || s.PrefetchAhead > 8 {
		s.PrefetchAhead = d.PrefetchAhead
	}
	if s.MaxEntryBytes < 1<<20 {
		s.MaxEntryBytes = d.MaxEntryBytes
	}
	if s.MaxPixels < 1_000_000 {
		s.MaxPixels = d.MaxPixels
	}
	if s.MaxEntries < 1000 {
		s.MaxEntries = d.MaxEntries
	}
	if s.WindowWidth < 800 {
		s.WindowWidth = d.WindowWidth
	}
	if s.WindowHeight < 600 {
		s.WindowHeight = d.WindowHeight
	}
	if s.SelectedEntry < 0 {
		s.SelectedEntry = 0
	}
	if s.ScrollTop < 0 {
		s.ScrollTop = 0
	}
}

// Manager loads and saves settings atomically.
type Manager struct {
	mu   sync.Mutex
	path string
	cur  Settings
}

// NewManager loads settings from dir, falling back to defaults when the file is
// missing or unreadable.
func NewManager(dir string) (*Manager, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("settings: create dir: %w", err)
	}
	m := &Manager{path: filepath.Join(dir, "settings.json"), cur: Default()}
	data, err := os.ReadFile(m.path)
	switch {
	case err == nil:
		var s Settings
		if json.Unmarshal(data, &s) == nil {
			s.Normalize()
			m.cur = s
		}
	case errors.Is(err, os.ErrNotExist):
		// First run: keep defaults.
	default:
		return nil, fmt.Errorf("settings: read: %w", err)
	}
	return m, nil
}

// Path returns the settings file location.
func (m *Manager) Path() string { return m.path }

// Get returns a copy of the current settings.
func (m *Manager) Get() Settings {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cur
}

// Update applies fn to the current settings, normalizes the result and saves it.
func (m *Manager) Update(fn func(*Settings)) (Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn(&m.cur)
	m.cur.Normalize()
	if err := m.saveLocked(); err != nil {
		return m.cur, err
	}
	return m.cur, nil
}

// Save writes the current settings.
func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveLocked()
}

func (m *Manager) saveLocked() error {
	data, err := json.MarshalIndent(m.cur, "", "  ")
	if err != nil {
		return fmt.Errorf("settings: marshal: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(m.path), ".settings-*")
	if err != nil {
		return fmt.Errorf("settings: temp: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("settings: write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("settings: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("settings: close: %w", err)
	}
	if err := os.Rename(name, m.path); err != nil {
		return fmt.Errorf("settings: rename: %w", err)
	}
	return nil
}
