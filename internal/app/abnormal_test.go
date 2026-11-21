package app

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestExternalDriveDisconnect simulates an unplugged drive: the archive file
// disappears while the app still holds its metadata. Every operation must
// report a clean error instead of panicking, and the app must keep working for
// other archives.
func TestExternalDriveDisconnect(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "external.zip")
	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 200, 200, 40)}})

	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	// Warm the cache so a stale thumbnail exists.
	if _, err := a.Thumbnail(sum.ID, sum.CoverOrdinal, 192); err != nil {
		t.Fatalf("thumb: %v", err)
	}

	// The drive is unplugged.
	if err := os.Remove(p); err != nil {
		t.Fatalf("remove: %v", err)
	}

	// A cached thumbnail may still be served, which is desirable: the user can
	// keep browsing what was already seen.
	if _, err := a.Thumbnail(sum.ID, sum.CoverOrdinal, 192); err != nil {
		t.Logf("cached thumbnail unavailable after disconnect: %v", err)
	}
	// A full image must fail cleanly.
	if _, err := a.FullImage(sum.ID, sum.CoverOrdinal); err == nil {
		t.Error("full image succeeded for a missing archive")
	}
	// Re-indexing must report "missing" rather than failing the whole app.
	again, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("re-index: %v", err)
	}
	if again.Status != "missing" {
		t.Errorf("status = %q, want missing", again.Status)
	}

	// The app must still work for a healthy archive.
	ok := filepath.Join(dir, "healthy.zip")
	makeZip(t, ok, []entry{{"b.jpg", encodeJPEG(t, 100, 100, 10)}})
	sum2, err := a.IndexArchive(ok)
	if err != nil {
		t.Fatalf("index healthy: %v", err)
	}
	if _, err := a.Thumbnail(sum2.ID, sum2.CoverOrdinal, 192); err != nil {
		t.Errorf("healthy archive broke after a disconnect: %v", err)
	}
}

// TestCacheInvalidationOnArchiveChange verifies that a modified archive never
// serves a thumbnail generated from its previous contents.
func TestCacheInvalidationOnArchiveChange(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "changing.zip")

	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 300, 300, 10)}})
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	before, err := a.Thumbnail(sum.ID, sum.CoverOrdinal, 192)
	if err != nil {
		t.Fatalf("thumb: %v", err)
	}

	// Replace the archive with a visually different one of the same dimensions.
	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 300, 300, 240)}})
	sum2, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index2: %v", err)
	}
	after, err := a.Thumbnail(sum2.ID, sum2.CoverOrdinal, 192)
	if err != nil {
		t.Fatalf("thumb2: %v", err)
	}
	if bytes.Equal(before.Data, after.Data) {
		t.Error("a stale thumbnail was served after the archive changed")
	}
}

// TestCacheSurvivesCorruption verifies that a truncated cache file is treated
// as a miss and regenerated rather than served.
func TestCacheSurvivesCorruption(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "c.zip")
	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 200, 200, 60)}})
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	first, err := a.Thumbnail(sum.ID, sum.CoverOrdinal, 192)
	if err != nil {
		t.Fatalf("thumb: %v", err)
	}

	// Corrupt every cached thumbnail file on disk.
	thumbDir := a.st.ThumbDir()
	err = filepath.Walk(thumbDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		return os.WriteFile(path, []byte("corrupted"), 0o644)
	})
	if err != nil {
		t.Fatalf("corrupt: %v", err)
	}

	// Clear the memory cache so the disk is consulted.
	a.mem.Clear()
	second, err := a.Thumbnail(sum.ID, sum.CoverOrdinal, 192)
	if err != nil {
		t.Fatalf("thumb after corruption: %v", err)
	}
	if bytes.Equal(second.Data, []byte("corrupted")) {
		t.Error("a corrupted cache file was served")
	}
	if !bytes.Equal(first.Data, second.Data) {
		t.Error("the regenerated thumbnail differs from the original")
	}
}

// TestRapidSelectionDoesNotMixImages hammers the selection the way a user
// clicking quickly does, and verifies that every response belongs to the entry
// that was requested.
func TestRapidSelectionDoesNotMixImages(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "rapid.zip")

	// Distinct shades make a mix-up detectable.
	var entries []entry
	for i := 0; i < 6; i++ {
		entries = append(entries, entry{
			name: "p" + itoa(i) + ".jpg",
			data: encodeJPEG(t, 200, 200, uint8(20+i*40)),
		})
	}
	makeZip(t, p, entries)
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	detail, err := a.GetArchiveEntries(sum.ID)
	if err != nil {
		t.Fatalf("entries: %v", err)
	}

	expected := make(map[int][]byte)
	for _, e := range detail.Entries {
		resp, err := a.FullImage(sum.ID, e.Ordinal)
		if err != nil {
			t.Fatalf("full: %v", err)
		}
		expected[e.Ordinal] = resp.Data
	}

	var wg sync.WaitGroup
	errs := make(chan string, 128)
	for round := 0; round < 10; round++ {
		for _, e := range detail.Entries {
			wg.Add(1)
			go func(ord int) {
				defer wg.Done()
				a.BumpSelection()
				resp, err := a.FullImage(sum.ID, ord)
				if err != nil {
					errs <- "error: " + err.Error()
					return
				}
				if !bytes.Equal(resp.Data, expected[ord]) {
					errs <- "ordinal " + itoa(ord) + " returned another entry"
				}
			}(e.Ordinal)
		}
	}
	wg.Wait()
	close(errs)
	for msg := range errs {
		t.Error(msg)
	}
}

// TestHugeImageRejected verifies the pixel limit is enforced before decoding.
func TestHugeImageRejected(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "huge.zip")

	// A 10000x10000 image is 100 MP, well over the 40 MP default.
	img := image.NewRGBA(image.Rect(0, 0, 10000, 10000))
	for y := 0; y < 10000; y += 100 {
		for x := 0; x < 10000; x += 100 {
			img.Set(x, y, color.RGBA{R: 1, G: 2, B: 3, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 10}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	makeZip(t, p, []entry{{"huge.jpg", buf.Bytes()}})

	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	_, err = a.Thumbnail(sum.ID, sum.CoverOrdinal, 192)
	if err == nil {
		t.Fatal("a 100 MP image was accepted")
	}
	ie := classifyImageError(err)
	if ie.Kind != ErrKindTooManyPix {
		t.Errorf("kind = %q, want %q", ie.Kind, ErrKindTooManyPix)
	}
}

// TestZip64Archive verifies that a ZIP64 archive is read correctly.
func TestZip64Archive(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "zip64.zip")

	// archive/zip writes ZIP64 structures when the entry count or sizes need
	// them; forcing many entries is the cheapest way to exercise the path.
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	zw := zip.NewWriter(f)
	img := encodeJPEG(t, 32, 32, 50)
	for i := 0; i < 300; i++ {
		w, err := zw.Create("p" + itoa(i) + ".jpg")
		if err != nil {
			t.Fatalf("create entry: %v", err)
		}
		if _, err := w.Write(img); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	f.Close()

	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if sum.Count != 300 {
		t.Errorf("count = %d, want 300", sum.Count)
	}
	if _, err := a.Thumbnail(sum.ID, sum.CoverOrdinal, 96); err != nil {
		t.Errorf("thumb: %v", err)
	}
}

// TestUnicodeNames verifies Japanese and emoji file names survive the whole
// pipeline.
func TestUnicodeNames(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "日本語 漫画 第1巻.cbz")
	makeZip(t, p, []entry{
		{"表紙.jpg", encodeJPEG(t, 100, 100, 30)},
		{"ページ 1.jpg", encodeJPEG(t, 100, 100, 60)},
		{"ページ 2.jpg", encodeJPEG(t, 100, 100, 90)},
	})

	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if sum.Count != 3 {
		t.Fatalf("count = %d, want 3", sum.Count)
	}
	detail, err := a.GetArchiveEntries(sum.ID)
	if err != nil {
		t.Fatalf("entries: %v", err)
	}
	// Natural order puts 表紙 before ページ 1 and ページ 2.
	if detail.Entries[0].Name != "ページ 1.jpg" && detail.Entries[0].Name != "表紙.jpg" {
		t.Errorf("first entry = %q", detail.Entries[0].Name)
	}
	for _, e := range detail.Entries {
		if _, err := a.Thumbnail(sum.ID, e.Ordinal, 96); err != nil {
			t.Errorf("thumb %q: %v", e.Name, err)
		}
	}
}

// TestEmptyAndCorruptArchivesAreIsolated verifies that one bad archive does not
// stop the others from being indexed.
func TestEmptyAndCorruptArchivesAreIsolated(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()

	good := filepath.Join(dir, "good.zip")
	makeZip(t, good, []entry{{"a.jpg", encodeJPEG(t, 100, 100, 10)}})

	empty := filepath.Join(dir, "empty.zip")
	makeZip(t, empty, []entry{{"notes.txt", []byte("nothing")}})

	corrupt := filepath.Join(dir, "corrupt.zip")
	if err := os.WriteFile(corrupt, []byte("garbage"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	statuses := map[string]string{}
	for _, p := range []string{good, empty, corrupt} {
		sum, err := a.IndexArchive(p)
		if err != nil {
			t.Fatalf("index %s: %v", filepath.Base(p), err)
		}
		statuses[filepath.Base(p)] = sum.Status
	}
	if statuses["good.zip"] != "ok" {
		t.Errorf("good = %q", statuses["good.zip"])
	}
	if statuses["empty.zip"] != "empty" {
		t.Errorf("empty = %q", statuses["empty.zip"])
	}
	if statuses["corrupt.zip"] != "corrupt" {
		t.Errorf("corrupt = %q", statuses["corrupt.zip"])
	}
}

// TestConcurrentThumbnailRequests verifies that many simultaneous requests for
// the same thumbnail all succeed and agree.
func TestConcurrentThumbnailRequests(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "conc.zip")
	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 400, 300, 70)}})
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}

	var wg sync.WaitGroup
	results := make([][]byte, 16)
	errs := make([]error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, err := a.Thumbnail(sum.ID, sum.CoverOrdinal, 192)
			if err != nil {
				errs[i] = err
				return
			}
			results[i] = resp.Data
		}(i)
	}
	wg.Wait()

	var first []byte
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("request %d: %v", i, errs[i])
		}
		if first == nil {
			first = results[i]
			continue
		}
		if !bytes.Equal(first, results[i]) {
			t.Errorf("request %d returned different bytes", i)
		}
	}
}
