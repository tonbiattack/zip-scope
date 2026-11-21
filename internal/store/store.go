// Package store persists archive metadata and thumbnails. Metadata lives in
// SQLite; thumbnail bytes live on disk under %LOCALAPPDATA%/ZipScope and are
// evicted least-recently-used once the configured budget is exceeded.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Status values recorded for an archive.
const (
	StatusOK        = "ok"
	StatusEmpty     = "empty"
	StatusCorrupt   = "corrupt"
	StatusEncrypted = "encrypted"
	StatusMissing   = "missing"
	StatusError     = "error"
)

// ArchiveRow is the cached metadata for one archive.
type ArchiveRow struct {
	ID          int64  `json:"id"`
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	MTime       int64  `json:"mtime"`
	Fingerprint string `json:"fingerprint"`
	Status      string `json:"status"`
	Count       int    `json:"count"`
	UpdatedAt   int64  `json:"updatedAt"`
}

// EntryRow is the cached metadata for one image entry.
type EntryRow struct {
	ArchiveID        int64  `json:"archiveId"`
	Ordinal          int    `json:"ordinal"`
	Name             string `json:"name"`
	CRC32            uint32 `json:"crc32"`
	CompressedSize   uint64 `json:"compressedSize"`
	UncompressedSize uint64 `json:"uncompressedSize"`
}

// ThumbRow is the cached metadata for one thumbnail file.
type ThumbRow struct {
	Key        string `json:"key"`
	Path       string `json:"path"`
	Bytes      int64  `json:"bytes"`
	LastAccess int64  `json:"lastAccess"`
}

// Store owns the SQLite database and the thumbnail directory.
type Store struct {
	db      *sql.DB
	root    string
	thumb   string
	mu      sync.Mutex
	limit   int64
	lastGC  time.Time
	closed  bool
	writeMu sync.Mutex
}

// Open creates or opens the store rooted at dir.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("store: create root: %w", err)
	}
	thumbDir := filepath.Join(dir, "thumbs")
	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		return nil, fmt.Errorf("store: create thumb dir: %w", err)
	}

	dbPath := filepath.Join(dir, "index.db")
	// WAL keeps reads from blocking the writer, and busy_timeout absorbs the
	// brief contention between the UI and background workers.
	dsn := "file:" + filepath.ToSlash(dbPath) +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open db: %w", err)
	}
	// modernc's driver is safe for concurrent use, but a small pool keeps
	// memory predictable and avoids SQLITE_BUSY storms.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)

	s := &Store{db: db, root: dir, thumb: thumbDir, limit: 2 << 30}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS archives (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	path        TEXT    NOT NULL UNIQUE,
	size        INTEGER NOT NULL,
	mtime       INTEGER NOT NULL,
	fingerprint TEXT    NOT NULL DEFAULT '',
	status      TEXT    NOT NULL DEFAULT 'ok',
	count       INTEGER NOT NULL DEFAULT 0,
	updated_at  INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS entries (
	archive_id        INTEGER NOT NULL REFERENCES archives(id) ON DELETE CASCADE,
	ordinal           INTEGER NOT NULL,
	name              TEXT    NOT NULL,
	crc32             INTEGER NOT NULL,
	compressed_size   INTEGER NOT NULL,
	uncompressed_size INTEGER NOT NULL,
	PRIMARY KEY (archive_id, ordinal)
);
CREATE TABLE IF NOT EXISTS thumbnails (
	key         TEXT    PRIMARY KEY,
	path        TEXT    NOT NULL,
	bytes       INTEGER NOT NULL,
	last_access INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_thumbnails_access ON thumbnails(last_access);
CREATE TABLE IF NOT EXISTS meta (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	return nil
}

// Root returns the store's base directory.
func (s *Store) Root() string { return s.root }

// ThumbDir returns the directory holding thumbnail files.
func (s *Store) ThumbDir() string { return s.thumb }

// Close releases the database handle.
func (s *Store) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	return s.db.Close()
}

// GetArchive looks up cached metadata for path.
func (s *Store) GetArchive(path string) (*ArchiveRow, error) {
	row := s.db.QueryRow(
		`SELECT id, path, size, mtime, fingerprint, status, count, updated_at FROM archives WHERE path = ?`, path)
	var a ArchiveRow
	err := row.Scan(&a.ID, &a.Path, &a.Size, &a.MTime, &a.Fingerprint, &a.Status, &a.Count, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get archive: %w", err)
	}
	return &a, nil
}

// GetArchiveByID looks up cached metadata by its row id.
func (s *Store) GetArchiveByID(id int64) (*ArchiveRow, error) {
	row := s.db.QueryRow(
		`SELECT id, path, size, mtime, fingerprint, status, count, updated_at FROM archives WHERE id = ?`, id)
	var a ArchiveRow
	err := row.Scan(&a.ID, &a.Path, &a.Size, &a.MTime, &a.Fingerprint, &a.Status, &a.Count, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get archive by id: %w", err)
	}
	return &a, nil
}

// PutArchive inserts or updates an archive row and replaces its entries in one
// transaction, so a crash can never leave a half-written listing.
func (s *Store) PutArchive(a ArchiveRow, entries []EntryRow) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	if a.UpdatedAt == 0 {
		a.UpdatedAt = now
	}
	if _, err := tx.Exec(`
INSERT INTO archives (path, size, mtime, fingerprint, status, count, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(path) DO UPDATE SET
	size = excluded.size,
	mtime = excluded.mtime,
	fingerprint = excluded.fingerprint,
	status = excluded.status,
	count = excluded.count,
	updated_at = excluded.updated_at`,
		a.Path, a.Size, a.MTime, a.Fingerprint, a.Status, a.Count, a.UpdatedAt); err != nil {
		return fmt.Errorf("store: upsert archive: %w", err)
	}

	var id int64
	if err := tx.QueryRow(`SELECT id FROM archives WHERE path = ?`, a.Path).Scan(&id); err != nil {
		return fmt.Errorf("store: archive id: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM entries WHERE archive_id = ?`, id); err != nil {
		return fmt.Errorf("store: clear entries: %w", err)
	}
	if len(entries) > 0 {
		stmt, err := tx.Prepare(`INSERT INTO entries
(archive_id, ordinal, name, crc32, compressed_size, uncompressed_size) VALUES (?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return fmt.Errorf("store: prepare entries: %w", err)
		}
		defer stmt.Close()
		for _, e := range entries {
			if _, err := stmt.Exec(id, e.Ordinal, e.Name, e.CRC32, e.CompressedSize, e.UncompressedSize); err != nil {
				return fmt.Errorf("store: insert entry: %w", err)
			}
		}
	}
	return tx.Commit()
}

// Entries returns the cached entries for an archive id, in natural order.
func (s *Store) Entries(archiveID int64) ([]EntryRow, error) {
	rows, err := s.db.Query(
		`SELECT archive_id, ordinal, name, crc32, compressed_size, uncompressed_size
		 FROM entries WHERE archive_id = ? ORDER BY ordinal`, archiveID)
	if err != nil {
		return nil, fmt.Errorf("store: entries: %w", err)
	}
	defer rows.Close()

	var out []EntryRow
	for rows.Next() {
		var e EntryRow
		if err := rows.Scan(&e.ArchiveID, &e.Ordinal, &e.Name, &e.CRC32, &e.CompressedSize, &e.UncompressedSize); err != nil {
			return nil, fmt.Errorf("store: scan entry: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AllArchivePaths returns every cached archive path, used to prune rows whose
// files disappeared.
func (s *Store) AllArchivePaths() ([]string, error) {
	rows, err := s.db.Query(`SELECT path FROM archives`)
	if err != nil {
		return nil, fmt.Errorf("store: all paths: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteArchive removes an archive and its entries.
func (s *Store) DeleteArchive(path string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.Exec(`DELETE FROM archives WHERE path = ?`, path)
	return err
}

// CountArchives returns how many archives are cached.
func (s *Store) CountArchives() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM archives`).Scan(&n)
	return n, err
}

// SetLimit updates the thumbnail byte budget.
func (s *Store) SetLimit(bytes int64) {
	s.mu.Lock()
	s.limit = bytes
	s.mu.Unlock()
}

// Limit returns the current thumbnail byte budget.
func (s *Store) Limit() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.limit
}

// ThumbPath returns the on-disk path for a thumbnail key, creating its shard
// directory.
func (s *Store) ThumbPath(key, shard, ext string) (string, error) {
	dir := filepath.Join(s.thumb, filepath.FromSlash(shard))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("store: thumb dir: %w", err)
	}
	return filepath.Join(dir, key+ext), nil
}

// LookupThumb returns the cached thumbnail bytes for key, refreshing its LRU
// timestamp. A missing file is reported as a miss and the row is dropped.
func (s *Store) LookupThumb(key string) ([]byte, bool) {
	var path string
	var size int64
	err := s.db.QueryRow(`SELECT path, bytes FROM thumbnails WHERE key = ?`, key).Scan(&path, &size)
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		// The row outlived its file: forget it so the next request regenerates.
		s.db.Exec(`DELETE FROM thumbnails WHERE key = ?`, key)
		return nil, false
	}
	s.db.Exec(`UPDATE thumbnails SET last_access = ? WHERE key = ?`, time.Now().Unix(), key)
	return data, true
}

// PutThumb writes a thumbnail atomically and records it. The temporary file is
// created in the destination directory so the rename stays on one volume.
func (s *Store) PutThumb(key, shard, ext string, data []byte) (string, error) {
	dest, err := s.ThumbPath(key, shard, ext)
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("store: temp thumb: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", fmt.Errorf("store: write thumb: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", fmt.Errorf("store: sync thumb: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("store: close thumb: %w", err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		// Two writers can race for the same key. On Windows the loser gets a
		// sharing violation; if the destination now exists and is readable, the
		// other writer won and its bytes are just as valid.
		if existing, rerr := os.ReadFile(dest); rerr == nil && len(existing) > 0 {
			return dest, nil
		}
		return "", fmt.Errorf("store: rename thumb: %w", err)
	}

	s.writeMu.Lock()
	_, err = s.db.Exec(`
INSERT INTO thumbnails (key, path, bytes, last_access) VALUES (?, ?, ?, ?)
ON CONFLICT(key) DO UPDATE SET path = excluded.path, bytes = excluded.bytes, last_access = excluded.last_access`,
		key, dest, len(data), time.Now().Unix())
	s.writeMu.Unlock()
	if err != nil {
		return "", fmt.Errorf("store: record thumb: %w", err)
	}
	return dest, nil
}

// ForgetThumb drops a thumbnail row and its file. It is used when the cached
// bytes turn out to be unusable.
func (s *Store) ForgetThumb(key string) {
	var path string
	if err := s.db.QueryRow(`SELECT path FROM thumbnails WHERE key = ?`, key).Scan(&path); err == nil {
		os.Remove(path)
	}
	s.writeMu.Lock()
	s.db.Exec(`DELETE FROM thumbnails WHERE key = ?`, key)
	s.writeMu.Unlock()
}

// ThumbStats reports the number of thumbnails and their total size.
func (s *Store) ThumbStats() (count int, bytes int64, err error) {
	err = s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(bytes), 0) FROM thumbnails`).Scan(&count, &bytes)
	return count, bytes, err
}

// Evict removes least-recently-used thumbnails until the total is within the
// budget. It is safe to call often; it self-throttles to once every 30s unless
// force is set.
func (s *Store) Evict(force bool) (removed int, err error) {
	s.mu.Lock()
	if !force && time.Since(s.lastGC) < 30*time.Second {
		s.mu.Unlock()
		return 0, nil
	}
	s.lastGC = time.Now()
	limit := s.limit
	s.mu.Unlock()

	_, total, err := s.ThumbStats()
	if err != nil {
		return 0, err
	}
	if total <= limit {
		return 0, nil
	}

	rows, err := s.db.Query(`SELECT key, path, bytes FROM thumbnails ORDER BY last_access ASC`)
	if err != nil {
		return 0, fmt.Errorf("store: evict query: %w", err)
	}
	type victim struct {
		key, path string
		bytes     int64
	}
	var victims []victim
	for rows.Next() {
		var v victim
		if err := rows.Scan(&v.key, &v.path, &v.bytes); err != nil {
			rows.Close()
			return 0, err
		}
		victims = append(victims, v)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, v := range victims {
		if total <= limit {
			break
		}
		// A missing file still frees its row, so the accounting stays honest.
		os.Remove(v.path)
		if _, err := s.db.Exec(`DELETE FROM thumbnails WHERE key = ?`, v.key); err != nil {
			return removed, err
		}
		total -= v.bytes
		removed++
	}
	return removed, nil
}

// ClearThumbs deletes every cached thumbnail and its rows.
func (s *Store) ClearThumbs() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := s.db.Exec(`DELETE FROM thumbnails`); err != nil {
		return fmt.Errorf("store: clear thumbs: %w", err)
	}
	// Remove the shard tree but keep the directory itself.
	entries, err := os.ReadDir(s.thumb)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		os.RemoveAll(filepath.Join(s.thumb, e.Name()))
	}
	return nil
}

// ClearAll drops every cached archive and thumbnail.
func (s *Store) ClearAll() error {
	if err := s.ClearThumbs(); err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.Exec(`DELETE FROM archives`)
	return err
}

// SweepOrphans deletes thumbnail files that have no database row, which can
// happen if the process dies between the rename and the insert.
func (s *Store) SweepOrphans() (int, error) {
	known := make(map[string]bool, 1024)
	rows, err := s.db.Query(`SELECT path FROM thumbnails`)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return 0, err
		}
		known[filepath.Clean(p)] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	removed := 0
	err = filepath.WalkDir(s.thumb, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if known[filepath.Clean(p)] {
			return nil
		}
		if os.Remove(p) == nil {
			removed++
		}
		return nil
	})
	return removed, err
}
