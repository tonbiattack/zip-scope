package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tonbiattack/zip-scope/internal/archive"
	"github.com/tonbiattack/zip-scope/internal/scan"
	"github.com/tonbiattack/zip-scope/internal/settings"
	"github.com/tonbiattack/zip-scope/internal/store"
)

// Event names emitted to the frontend.
const (
	// EventScanStarted fires when a scan begins.
	EventScanStarted = "scan:started"
	// EventScanFile fires for every archive discovered, so the list fills in
	// progressively instead of waiting for the whole walk.
	EventScanFile = "scan:file"
	// EventScanDone fires once the walk finishes or is cancelled.
	EventScanDone = "scan:done"
	// EventScanSkip fires for an unreadable path.
	EventScanSkip = "scan:skip"
)

// ArchiveSummary is one row in the left-hand list.
type ArchiveSummary struct {
	// ID is the app-assigned identifier used in image URLs.
	ID int64 `json:"id"`
	// Path is the absolute filesystem path.
	Path string `json:"path"`
	// Name is the file name without its directory.
	Name string `json:"name"`
	// Size is the archive size in bytes.
	Size int64 `json:"size"`
	// ModTime is the archive modification time in Unix milliseconds.
	ModTime int64 `json:"modTime"`
	// Count is the number of image entries.
	Count int `json:"count"`
	// Status is "ok", "empty", "corrupt", "encrypted", "missing" or "error".
	Status string `json:"status"`
	// Error carries a human-readable reason when Status is not "ok".
	Error string `json:"error,omitempty"`
	// CoverOrdinal is the representative image's ordinal, or -1.
	CoverOrdinal int `json:"coverOrdinal"`
	// CoverWidth is the thumbnail width to request for the cover.
	CoverWidth int `json:"coverWidth"`
}

// scanState tracks the running scan so a new one can cancel the old one.
type scanState struct {
	cancel context.CancelFunc
	root   string
	gen    int64
}

// ScanResult summarises a finished scan.
type ScanResult struct {
	Root      string `json:"root"`
	Files     int    `json:"files"`
	Dirs      int    `json:"dirs"`
	Skipped   int    `json:"skipped"`
	Cancelled bool   `json:"cancelled"`
	Duration  int64  `json:"durationMs"`
	Gen       int64  `json:"gen"`
}

// StartScan begins an asynchronous recursive scan of root. Results stream to
// the frontend through events; the returned generation identifies this scan.
func (a *App) StartScan(root string) (int64, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return 0, fmt.Errorf("app: empty root")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return 0, fmt.Errorf("app: resolve root: %w", err)
	}
	if _, err := filepath.EvalSymlinks(abs); err != nil {
		return 0, fmt.Errorf("app: %w", err)
	}

	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return 0, fmt.Errorf("app: shutting down")
	}
	if a.scan != nil && a.scan.cancel != nil {
		a.scan.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	gen := time.Now().UnixNano()
	a.scan = &scanState{cancel: cancel, root: abs, gen: gen}
	a.mu.Unlock()

	// Persist the root immediately so a crash still restores it.
	a.cfg.Update(func(s *settings.Settings) { s.Root = abs })

	go a.runScan(ctx, abs, gen)
	return gen, nil
}

// CancelScan stops the running scan, if any.
func (a *App) CancelScan() {
	a.mu.Lock()
	if a.scan != nil && a.scan.cancel != nil {
		a.scan.cancel()
	}
	a.mu.Unlock()
}

func (a *App) runScan(ctx context.Context, root string, gen int64) {
	a.emit(EventScanStarted, map[string]any{"root": root, "gen": gen})

	// The scan runs on its own goroutine; the callback only enqueues work, so a
	// slow UI never blocks the walk.
	var mu sync.Mutex
	var found []scan.File
	opts := scan.Options{
		OnFile: func(f scan.File) {
			mu.Lock()
			found = append(found, f)
			mu.Unlock()
			// Index the archive off the walk goroutine so the walk stays fast.
			a.indexAndEmit(f, gen)
		},
		OnSkip: func(s scan.Skip) {
			a.emit(EventScanSkip, map[string]any{"path": s.Path, "err": s.Err, "gen": gen})
		},
	}

	stats, err := scan.Scan(ctx, root, opts)
	res := ScanResult{
		Root:      root,
		Files:     stats.Files,
		Dirs:      stats.Dirs,
		Skipped:   stats.Skipped,
		Cancelled: stats.Cancelled,
		Duration:  stats.Duration.Milliseconds(),
		Gen:       gen,
	}
	payload := map[string]any{"result": res}
	if err != nil {
		payload["error"] = err.Error()
	}
	a.emit(EventScanDone, payload)
}

// indexAndEmit indexes one archive and pushes its summary to the UI. Indexing
// is cheap (central directory only) and is cached in SQLite.
func (a *App) indexAndEmit(f scan.File, gen int64) {
	sum, err := a.IndexArchive(f.Path)
	if err != nil {
		a.emit(EventScanFile, map[string]any{
			"archive": ArchiveSummary{
				Path:   f.Path,
				Name:   filepath.Base(f.Path),
				Size:   f.Size,
				Status: store.StatusError,
				Error:  err.Error(),
			},
			"gen": gen,
		})
		return
	}
	a.emit(EventScanFile, map[string]any{"archive": sum, "gen": gen})
}

// IndexArchive opens an archive's central directory (or reuses the cached
// listing when the fingerprint still matches) and returns its summary.
func (a *App) IndexArchive(path string) (ArchiveSummary, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ArchiveSummary{}, err
	}
	fi, err := osStat(abs)
	if err != nil {
		return ArchiveSummary{Path: abs, Name: filepath.Base(abs), Status: store.StatusMissing, Error: err.Error()}, nil
	}

	limits := a.limits()
	arc, err := archive.Open(abs, limits)
	if err != nil {
		status := store.StatusCorrupt
		if isPermission(err) {
			status = store.StatusError
		}
		row := store.ArchiveRow{
			Path: abs, Size: fi.Size(), MTime: fi.ModTime().Unix(),
			Status: status, Count: 0,
		}
		a.st.PutArchive(row, nil)
		return ArchiveSummary{
			Path: abs, Name: filepath.Base(abs), Size: fi.Size(),
			ModTime: fi.ModTime().UnixMilli(), Status: status, Error: err.Error(),
			CoverOrdinal: -1,
		}, nil
	}

	status := store.StatusOK
	if arc.Count() == 0 {
		status = store.StatusEmpty
	}

	// Reuse the cached row when nothing changed, which avoids rewriting the
	// entry table on every launch.
	row, _ := a.st.GetArchive(abs)
	if row == nil || row.Fingerprint != arc.Fingerprint() || row.Size != fi.Size() ||
		row.MTime != fi.ModTime().Unix() || row.Status != status {
		entries := make([]store.EntryRow, 0, arc.Count())
		for _, e := range arc.Entries() {
			entries = append(entries, store.EntryRow{
				Ordinal:          e.Ordinal,
				Name:             e.Name,
				CRC32:            e.CRC32,
				CompressedSize:   e.CompressedSize,
				UncompressedSize: e.UncompressedSize,
			})
		}
		newRow := store.ArchiveRow{
			Path: abs, Size: fi.Size(), MTime: fi.ModTime().Unix(),
			Fingerprint: arc.Fingerprint(), Status: status, Count: arc.Count(),
		}
		if err := a.st.PutArchive(newRow, entries); err != nil {
			return ArchiveSummary{}, err
		}
		row, _ = a.st.GetArchive(abs)
	}

	id := int64(0)
	if row != nil {
		id = row.ID
	}

	cover := -1
	if arc.Count() > 0 {
		cover = arc.Entries()[0].Ordinal
	}

	// Register the handle so image requests can resolve the id.
	a.mu.Lock()
	if existing, ok := a.archives[id]; ok {
		existing.arc = arc
		existing.lastUse = time.Now()
	} else {
		a.archives[id] = &openArchive{id: id, path: abs, arc: arc, opened: time.Now(), lastUse: time.Now()}
	}
	a.mu.Unlock()

	return ArchiveSummary{
		ID:           id,
		Path:         abs,
		Name:         filepath.Base(abs),
		Size:         fi.Size(),
		ModTime:      fi.ModTime().UnixMilli(),
		Count:        arc.Count(),
		Status:       status,
		CoverOrdinal: cover,
		CoverWidth:   snapWidth(a.cfg.Get().ThumbSize),
	}, nil
}

// EntryInfo is one image inside the selected archive.
type EntryInfo struct {
	Ordinal int    `json:"ordinal"`
	Name    string `json:"name"`
	CRC32   uint32 `json:"crc32"`
	Size    uint64 `json:"size"`
	// Encrypted marks entries that cannot be decoded.
	Encrypted bool `json:"encrypted"`
}

// ArchiveDetail is the payload for the grid.
type ArchiveDetail struct {
	ID      int64       `json:"id"`
	Path    string      `json:"path"`
	Name    string      `json:"name"`
	Status  string      `json:"status"`
	Error   string      `json:"error,omitempty"`
	Entries []EntryInfo `json:"entries"`
}

// GetArchiveEntries returns the image entries of an archive in natural order.
func (a *App) GetArchiveEntries(id int64) (ArchiveDetail, error) {
	oa, err := a.lookup(id)
	if err != nil {
		return ArchiveDetail{}, err
	}
	arc := oa.arc
	out := ArchiveDetail{
		ID:      id,
		Path:    oa.path,
		Name:    filepath.Base(oa.path),
		Status:  store.StatusOK,
		Entries: make([]EntryInfo, 0, arc.Count()),
	}
	if arc.Count() == 0 {
		out.Status = store.StatusEmpty
	}
	for _, e := range arc.Entries() {
		out.Entries = append(out.Entries, EntryInfo{
			Ordinal:   e.Ordinal,
			Name:      e.Name,
			CRC32:     e.CRC32,
			Size:      e.UncompressedSize,
			Encrypted: e.Encrypted,
		})
	}
	return out, nil
}

// lookup resolves an archive id to its open handle, reopening it if the cache
// was cleared.
func (a *App) lookup(id int64) (*openArchive, error) {
	a.mu.Lock()
	oa, ok := a.archives[id]
	if ok {
		oa.lastUse = time.Now()
	}
	a.mu.Unlock()
	if ok {
		return oa, nil
	}

	// Reopen from the database so a page reload does not lose the selection.
	row, err := a.st.GetArchiveByID(id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, fmt.Errorf("app: unknown archive id %d", id)
	}
	arc, err := archive.Open(row.Path, a.limits())
	if err != nil {
		return nil, err
	}
	oa = &openArchive{id: id, path: row.Path, arc: arc, opened: time.Now(), lastUse: time.Now()}
	a.mu.Lock()
	a.archives[id] = oa
	a.mu.Unlock()
	return oa, nil
}

// SortMode values accepted by SortArchives.
const (
	SortByName  = "name"
	SortByMTime = "mtime"
	SortBySize  = "size"
)

// SortArchives orders summaries in place. It is exported so the frontend can
// sort without a round trip, and used by tests.
func SortArchives(list []ArchiveSummary, mode string, desc bool) {
	less := func(i, j int) bool { return list[i].Name < list[j].Name }
	switch mode {
	case SortByMTime:
		less = func(i, j int) bool {
			if list[i].ModTime != list[j].ModTime {
				return list[i].ModTime > list[j].ModTime
			}
			return list[i].Name < list[j].Name
		}
	case SortBySize:
		less = func(i, j int) bool {
			if list[i].Size != list[j].Size {
				return list[i].Size > list[j].Size
			}
			return list[i].Name < list[j].Name
		}
	default:
		less = func(i, j int) bool {
			if c := archive.NaturalCompare(list[i].Name, list[j].Name); c != 0 {
				return c < 0
			}
			return list[i].Path < list[j].Path
		}
	}
	sort.SliceStable(list, func(i, j int) bool {
		if desc {
			return less(j, i)
		}
		return less(i, j)
	})
}
