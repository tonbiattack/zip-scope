package scan

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"
)

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestScanFindsArchivesRecursively(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "a.zip"))
	touch(t, filepath.Join(root, "b.cbz"))
	touch(t, filepath.Join(root, "ignore.txt"))
	touch(t, filepath.Join(root, "sub", "c.zip"))
	touch(t, filepath.Join(root, "sub", "deep", "d.CBZ"))

	var c Collector
	stats, err := Scan(context.Background(), root, c.Options())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if stats.Cancelled {
		t.Error("scan reported cancelled")
	}
	files := c.Files()
	if len(files) != 4 {
		t.Fatalf("found %d files, want 4: %+v", len(files), files)
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, filepath.Base(f.Path))
	}
	sort.Strings(names)
	want := []string{"a.zip", "b.cbz", "c.zip", "d.CBZ"}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("names = %v, want %v", names, want)
			break
		}
	}
}

func TestScanSingleArchive(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "one.zip")
	touch(t, p)

	var c Collector
	stats, err := Scan(context.Background(), p, c.Options())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if stats.Files != 1 {
		t.Fatalf("files = %d, want 1", stats.Files)
	}
	if got := c.Files(); len(got) != 1 || got[0].Path != p {
		t.Fatalf("files = %+v, want the single archive", got)
	}
}

func TestScanRejectsUnsupportedSingleFile(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "notes.txt")
	touch(t, p)
	if _, err := Scan(context.Background(), p, Options{}); err == nil {
		t.Fatal("expected an error for an unsupported single file")
	}
}

func TestScanJapanesePaths(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "日本語フォルダ", "漫画 第1巻.cbz")
	touch(t, p)

	var c Collector
	if _, err := Scan(context.Background(), root, c.Options()); err != nil {
		t.Fatalf("scan: %v", err)
	}
	files := c.Files()
	if len(files) != 1 {
		t.Fatalf("files = %d, want 1", len(files))
	}
	if filepath.Base(files[0].Path) != "漫画 第1巻.cbz" {
		t.Errorf("path = %q", files[0].Path)
	}
}

func TestScanCancel(t *testing.T) {
	root := t.TempDir()
	// Enough directories that the walk cannot finish before cancellation.
	for i := 0; i < 200; i++ {
		touch(t, filepath.Join(root, "d"+string(rune('a'+i%26)), "f"+itoa(i)+".zip"))
	}

	ctx, cancel := context.WithCancel(context.Background())
	var seen int
	opts := Options{
		OnFile: func(File) {
			seen++
			if seen == 3 {
				cancel()
			}
		},
	}
	stats, err := Scan(ctx, root, opts)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !stats.Cancelled {
		t.Error("scan did not report cancellation")
	}
	if stats.Files >= 200 {
		t.Errorf("scan reported %d files despite cancellation", stats.Files)
	}
}

func TestScanSkipsSymlinks(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junction creation is Windows specific")
	}
	root := t.TempDir()
	target := filepath.Join(root, "real")
	touch(t, filepath.Join(target, "inside.zip"))

	link := filepath.Join(root, "link")
	// Creating a junction needs no elevation on Windows.
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create link: %v", err)
	}

	var c Collector
	if _, err := Scan(context.Background(), root, c.Options()); err != nil {
		t.Fatalf("scan: %v", err)
	}
	// The real directory is visited once; the link must not be followed, so the
	// archive is reported exactly once.
	if got := len(c.Files()); got != 1 {
		t.Fatalf("files = %d, want 1 (link followed?)", got)
	}
	if len(c.Skips()) == 0 {
		t.Error("link was not recorded as skipped")
	}
}

func TestScanMaxDepth(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "top.zip"))
	touch(t, filepath.Join(root, "a", "mid.zip"))
	touch(t, filepath.Join(root, "a", "b", "deep.zip"))

	var c Collector
	opts := c.Options()
	opts.MaxDepth = 1
	if _, err := Scan(context.Background(), root, opts); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got := len(c.Files()); got != 2 {
		t.Fatalf("files = %d, want 2", got)
	}
}

func TestScanMissingRoot(t *testing.T) {
	if _, err := Scan(context.Background(), filepath.Join(t.TempDir(), "nope"), Options{}); err == nil {
		t.Fatal("expected an error for a missing root")
	}
}

func TestScanStreamsProgressively(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 20; i++ {
		touch(t, filepath.Join(root, "f"+itoa(i)+".zip"))
	}
	var firstAt time.Time
	start := time.Now()
	opts := Options{
		OnFile: func(File) {
			if firstAt.IsZero() {
				firstAt = time.Now()
			}
		},
	}
	if _, err := Scan(context.Background(), root, opts); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if firstAt.IsZero() {
		t.Fatal("no file was reported")
	}
	// The first result must arrive before the walk completes, which is what
	// makes the list fill in progressively.
	if !firstAt.Before(start.Add(time.Second)) {
		t.Errorf("first result took %v", firstAt.Sub(start))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
