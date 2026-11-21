// Package app wires the scanner, archive reader, cache and prefetcher into the
// API the Wails frontend calls.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tonbiattack/zip-scope/internal/archive"
	"github.com/tonbiattack/zip-scope/internal/prefetch"
	"github.com/tonbiattack/zip-scope/internal/settings"
	"github.com/tonbiattack/zip-scope/internal/store"
	"github.com/tonbiattack/zip-scope/internal/thumb"
)

// App is the bound object. Every exported method is callable from the frontend.
type App struct {
	ctx context.Context

	cfg   *settings.Manager
	st    *store.Store
	sched *prefetch.Scheduler
	mem   *prefetch.MemoryCache
	work  *prefetch.Weighted
	// flight collapses concurrent requests for the same image.
	flight *flightGroup

	mu       sync.Mutex
	archives map[int64]*openArchive
	scan     *scanState
	closed   bool
}

// openArchive is a cached handle on one indexed archive.
type openArchive struct {
	id      int64
	path    string
	arc     *archive.Archive
	opened  time.Time
	lastUse time.Time
}

// New creates the application. dir is the per-user data directory.
func New(dir string) (*App, error) {
	cfg, err := settings.NewManager(dir)
	if err != nil {
		return nil, err
	}
	st, err := store.Open(dir)
	if err != nil {
		return nil, err
	}
	cur := cfg.Get()
	st.SetLimit(cur.CacheLimitBytes)

	a := &App{
		cfg:      cfg,
		st:       st,
		sched:    prefetch.NewScheduler(cur.Workers),
		mem:      prefetch.NewMemoryCache(cur.MemoryLimitBytes),
		work:     prefetch.NewWeighted(cur.MemoryLimitBytes),
		flight:   newFlightGroup(),
		archives: make(map[int64]*openArchive),
	}
	// Reclaim space left behind by a previous run that died mid-write.
	go func() {
		st.SweepOrphans()
		st.Evict(true)
	}()
	return a, nil
}

// Startup is called by Wails once the runtime context exists.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

// Shutdown releases background workers and the database.
func (a *App) Shutdown(ctx context.Context) {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	a.closed = true
	if a.scan != nil && a.scan.cancel != nil {
		a.scan.cancel()
	}
	a.mu.Unlock()

	a.sched.Close()
	a.st.Close()
}

// limits returns the archive limits from the current settings.
func (a *App) limits() archive.Limits {
	s := a.cfg.Get()
	return archive.Limits{
		MaxEntryBytes: s.MaxEntryBytes,
		MaxPixels:     s.MaxPixels,
		MaxEntries:    s.MaxEntries,
	}
}

// GetSettings returns the persisted settings.
func (a *App) GetSettings() settings.Settings {
	return a.cfg.Get()
}

// SettingsPatch is a partial settings update. Nil fields are left unchanged.
type SettingsPatch struct {
	Root             *string  `json:"root"`
	SelectedArchive  *string  `json:"selectedArchive"`
	SelectedEntry    *int     `json:"selectedEntry"`
	ScrollTop        *float64 `json:"scrollTop"`
	ThumbSize        *int     `json:"thumbSize"`
	SortMode         *string  `json:"sortMode"`
	SortDesc         *bool    `json:"sortDesc"`
	Search           *string  `json:"search"`
	CacheLimitBytes  *int64   `json:"cacheLimitBytes"`
	MemoryLimitBytes *int64   `json:"memoryLimitBytes"`
	Workers          *int     `json:"workers"`
	PrefetchAhead    *int     `json:"prefetchAhead"`
	WindowWidth      *int     `json:"windowWidth"`
	WindowHeight     *int     `json:"windowHeight"`
}

// UpdateSettings applies a patch and returns the normalized settings.
func (a *App) UpdateSettings(p SettingsPatch) (settings.Settings, error) {
	updated, err := a.cfg.Update(func(s *settings.Settings) {
		if p.Root != nil {
			s.Root = *p.Root
		}
		if p.SelectedArchive != nil {
			s.SelectedArchive = *p.SelectedArchive
		}
		if p.SelectedEntry != nil {
			s.SelectedEntry = *p.SelectedEntry
		}
		if p.ScrollTop != nil {
			s.ScrollTop = *p.ScrollTop
		}
		if p.ThumbSize != nil {
			s.ThumbSize = *p.ThumbSize
		}
		if p.SortMode != nil {
			s.SortMode = *p.SortMode
		}
		if p.SortDesc != nil {
			s.SortDesc = *p.SortDesc
		}
		if p.Search != nil {
			s.Search = *p.Search
		}
		if p.CacheLimitBytes != nil {
			s.CacheLimitBytes = *p.CacheLimitBytes
		}
		if p.MemoryLimitBytes != nil {
			s.MemoryLimitBytes = *p.MemoryLimitBytes
		}
		if p.Workers != nil {
			s.Workers = *p.Workers
		}
		if p.PrefetchAhead != nil {
			s.PrefetchAhead = *p.PrefetchAhead
		}
		if p.WindowWidth != nil {
			s.WindowWidth = *p.WindowWidth
		}
		if p.WindowHeight != nil {
			s.WindowHeight = *p.WindowHeight
		}
	})
	if err != nil {
		return updated, err
	}
	// Apply the limits that can change at runtime.
	a.st.SetLimit(updated.CacheLimitBytes)
	a.mem.SetLimit(updated.MemoryLimitBytes)
	a.work.SetLimit(updated.MemoryLimitBytes)
	if updated.CacheLimitBytes != a.st.Limit() {
		go a.st.Evict(true)
	}
	return updated, nil
}

// CacheStats reports cache usage for the settings panel.
type CacheStats struct {
	ThumbCount   int    `json:"thumbCount"`
	ThumbBytes   int64  `json:"thumbBytes"`
	ThumbLimit   int64  `json:"thumbLimit"`
	ArchiveCount int    `json:"archiveCount"`
	MemoryUsed   int64  `json:"memoryUsed"`
	MemoryLimit  int64  `json:"memoryLimit"`
	MemoryItems  int    `json:"memoryItems"`
	Root         string `json:"root"`
}

// GetCacheStats returns current cache usage.
func (a *App) GetCacheStats() (CacheStats, error) {
	count, bytes, err := a.st.ThumbStats()
	if err != nil {
		return CacheStats{}, err
	}
	archives, err := a.st.CountArchives()
	if err != nil {
		return CacheStats{}, err
	}
	mem := a.mem.Stats()
	return CacheStats{
		ThumbCount:   count,
		ThumbBytes:   bytes,
		ThumbLimit:   a.st.Limit(),
		ArchiveCount: archives,
		MemoryUsed:   mem.Used,
		MemoryLimit:  mem.Limit,
		MemoryItems:  mem.Items,
		Root:         a.st.Root(),
	}, nil
}

// ClearCache deletes every cached thumbnail and archive row.
func (a *App) ClearCache() error {
	a.mu.Lock()
	a.archives = make(map[int64]*openArchive)
	a.mu.Unlock()
	a.mem.Clear()
	return a.st.ClearAll()
}

// ClearThumbnails deletes only the thumbnail files, keeping archive metadata.
func (a *App) ClearThumbnails() error {
	return a.st.ClearThumbs()
}

// EvictCache runs the LRU eviction immediately.
func (a *App) EvictCache() (int, error) {
	return a.st.Evict(true)
}

// PickFolder opens the native folder picker.
func (a *App) PickFolder() (string, error) {
	return a.pickFolder()
}

// PickArchive opens the native file picker for a single ZIP/CBZ.
func (a *App) PickArchive() (string, error) {
	return a.pickArchive()
}

// RevealInExplorer opens the containing folder for a path.
func (a *App) RevealInExplorer(path string) error {
	if path == "" {
		return errors.New("app: empty path")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("app: %w", err)
	}
	return revealPath(path)
}

// ThumbSizeOptions returns the widths the grid slider may use.
func (a *App) ThumbSizeOptions() []int {
	return []int{96, 128, 192, 256, 384, 512}
}

// snapWidth maps a requested width onto the thumbnail ladder.
func snapWidth(w int) int {
	return thumb.SnapSize(w)
}

// DataDir resolves the per-user data directory.
func DataDir() (string, error) {
	return dataDir()
}

// dataDir resolves the per-user data directory.
func dataDir() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		var err error
		base, err = os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("app: locate data dir: %w", err)
		}
	}
	dir := filepath.Join(base, "ZipScope")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("app: create data dir: %w", err)
	}
	return dir, nil
}
