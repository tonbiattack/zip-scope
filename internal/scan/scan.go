// Package scan walks a directory tree asynchronously and reports ZIP/CBZ files
// as it finds them. It never follows symbolic links or junctions, records
// access-denied directories instead of failing, and stops promptly when its
// context is cancelled.
package scan

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// File is one discovered archive.
type File struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
}

// Skip records a path that could not be read. The scan continues past it.
type Skip struct {
	Path string `json:"path"`
	Err  string `json:"err"`
}

// Stats summarises a completed or cancelled scan.
type Stats struct {
	// Dirs is the number of directories visited.
	Dirs int `json:"dirs"`
	// Files is the number of archives reported.
	Files int `json:"files"`
	// Skipped is the number of unreadable paths.
	Skipped int `json:"skipped"`
	// Cancelled reports that the context ended the walk early.
	Cancelled bool `json:"cancelled"`
	// Duration is the wall-clock time spent scanning.
	Duration time.Duration `json:"duration"`
}

// Options configures a scan.
type Options struct {
	// Extensions is the set of lower-case extensions to report, including the
	// leading dot. Nil means ".zip" and ".cbz".
	Extensions []string
	// OnFile is called for every archive found, in discovery order. It runs on
	// the scanning goroutine, so it must not block for long.
	OnFile func(File)
	// OnSkip is called for every unreadable path.
	OnSkip func(Skip)
	// MaxDepth limits recursion; zero means unlimited.
	MaxDepth int
}

func (o Options) extensions() map[string]bool {
	exts := o.Extensions
	if len(exts) == 0 {
		exts = []string{".zip", ".cbz"}
	}
	m := make(map[string]bool, len(exts))
	for _, e := range exts {
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		m[strings.ToLower(e)] = true
	}
	return m
}

// IsArchiveName reports whether name has one of the default archive
// extensions.
func IsArchiveName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".zip", ".cbz":
		return true
	}
	return false
}

// Scan walks root and reports archives. It returns when the walk finishes or
// ctx is cancelled; a cancelled scan returns the partial Stats with
// Cancelled set and a nil error.
func Scan(ctx context.Context, root string, opts Options) (Stats, error) {
	start := time.Now()
	stats := Stats{}
	exts := opts.extensions()

	root = filepath.Clean(root)
	fi, err := os.Stat(root)
	if err != nil {
		return stats, err
	}

	// A single archive can be scanned directly, which is what drag-and-drop of
	// one ZIP produces.
	if !fi.IsDir() {
		if exts[strings.ToLower(filepath.Ext(root))] {
			stats.Files = 1
			stats.Duration = time.Since(start)
			if opts.OnFile != nil {
				opts.OnFile(File{Path: root, Size: fi.Size(), ModTime: fi.ModTime()})
			}
			return stats, nil
		}
		return stats, errors.New("scan: not a directory or supported archive: " + root)
	}

	type dirItem struct {
		path  string
		depth int
	}
	stack := []dirItem{{path: root, depth: 0}}

	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			stats.Cancelled = true
			break
		}
		// Depth-first, popping from the end. Children are pushed in reverse
		// sorted order so the traversal is deterministic.
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		stats.Dirs++
		entries, err := os.ReadDir(cur.path)
		if err != nil {
			stats.Skipped++
			if opts.OnSkip != nil {
				opts.OnSkip(Skip{Path: cur.path, Err: err.Error()})
			}
			continue
		}

		var subdirs []dirItem
		for _, de := range entries {
			if err := ctx.Err(); err != nil {
				stats.Cancelled = true
				break
			}
			name := de.Name()
			full := filepath.Join(cur.path, name)

			// Never follow links or junctions: a reparse point could loop or
			// escape the chosen root.
			if de.Type()&fs.ModeSymlink != 0 {
				stats.Skipped++
				if opts.OnSkip != nil {
					opts.OnSkip(Skip{Path: full, Err: "symbolic link or junction not followed"})
				}
				continue
			}

			if de.IsDir() {
				if opts.MaxDepth > 0 && cur.depth+1 > opts.MaxDepth {
					continue
				}
				subdirs = append(subdirs, dirItem{path: full, depth: cur.depth + 1})
				continue
			}

			if !exts[strings.ToLower(filepath.Ext(name))] {
				continue
			}
			info, err := de.Info()
			if err != nil {
				stats.Skipped++
				if opts.OnSkip != nil {
					opts.OnSkip(Skip{Path: full, Err: err.Error()})
				}
				continue
			}
			stats.Files++
			if opts.OnFile != nil {
				opts.OnFile(File{Path: full, Size: info.Size(), ModTime: info.ModTime()})
			}
		}
		if stats.Cancelled {
			break
		}

		sort.Slice(subdirs, func(i, j int) bool { return subdirs[i].path > subdirs[j].path })
		stack = append(stack, subdirs...)
	}

	stats.Duration = time.Since(start)
	return stats, nil
}

// Collector is a concurrency-safe sink for scan results, useful when the caller
// wants the whole list rather than a stream.
type Collector struct {
	mu    sync.Mutex
	files []File
	skips []Skip
}

// Options returns scan options that feed this collector.
func (c *Collector) Options() Options {
	return Options{
		OnFile: func(f File) {
			c.mu.Lock()
			c.files = append(c.files, f)
			c.mu.Unlock()
		},
		OnSkip: func(s Skip) {
			c.mu.Lock()
			c.skips = append(c.skips, s)
			c.mu.Unlock()
		},
	}
}

// Files returns a copy of the collected archives.
func (c *Collector) Files() []File {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]File, len(c.files))
	copy(out, c.files)
	return out
}

// Skips returns a copy of the collected unreadable paths.
func (c *Collector) Skips() []Skip {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Skip, len(c.skips))
	copy(out, c.skips)
	return out
}
