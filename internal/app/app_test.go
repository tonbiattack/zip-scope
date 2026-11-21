package app

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// testApp builds an App rooted in a temporary directory.
func testApp(t *testing.T) *App {
	t.Helper()
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() { a.Shutdown(nil) })
	return a
}

func encodeJPEG(t *testing.T, w, h int, shade uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: shade, G: uint8(x % 256), B: uint8(y % 256), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{R: 30, G: 200, B: 120, A: uint8((x * 3) % 256)})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

type entry struct {
	name string
	data []byte
}

func makeZip(t *testing.T, path string, entries []entry) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatalf("create entry: %v", err)
		}
		if _, err := w.Write(e.data); err != nil {
			t.Fatalf("write entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestIndexArchiveAndEntries(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "book.cbz")
	makeZip(t, p, []entry{
		{"page2.jpg", encodeJPEG(t, 40, 30, 10)},
		{"page1.jpg", encodeJPEG(t, 40, 30, 20)},
		{"page10.jpg", encodeJPEG(t, 40, 30, 30)},
		{"readme.txt", []byte("hi")},
	})

	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if sum.Status != "ok" {
		t.Errorf("status = %q, want ok", sum.Status)
	}
	if sum.Count != 3 {
		t.Errorf("count = %d, want 3", sum.Count)
	}
	if sum.ID == 0 {
		t.Error("archive id was not assigned")
	}
	if sum.CoverOrdinal < 0 {
		t.Error("cover ordinal was not set")
	}

	detail, err := a.GetArchiveEntries(sum.ID)
	if err != nil {
		t.Fatalf("entries: %v", err)
	}
	want := []string{"page1.jpg", "page2.jpg", "page10.jpg"}
	for i, w := range want {
		if detail.Entries[i].Name != w {
			t.Errorf("entry %d = %q, want %q", i, detail.Entries[i].Name, w)
		}
	}
}

func TestThumbnailIsCachedAndReused(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "t.zip")
	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 400, 300, 50)}})

	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	ord := sum.CoverOrdinal

	first, err := a.Thumbnail(sum.ID, ord, 192)
	if err != nil {
		t.Fatalf("thumb: %v", err)
	}
	if first.Cached {
		t.Error("first thumbnail reported as cached")
	}
	if len(first.Data) == 0 {
		t.Fatal("empty thumbnail")
	}

	second, err := a.Thumbnail(sum.ID, ord, 192)
	if err != nil {
		t.Fatalf("thumb2: %v", err)
	}
	if !second.Cached {
		t.Error("second thumbnail was not served from cache")
	}
	if !bytes.Equal(first.Data, second.Data) {
		t.Error("cached thumbnail differs from the generated one")
	}
}

func TestThumbnailSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	zipPath := filepath.Join(dir, "persist.zip")
	makeZip(t, zipPath, []entry{{"a.jpg", encodeJPEG(t, 300, 200, 90)}})

	a1, err := New(dataDir)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	sum, err := a1.IndexArchive(zipPath)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	first, err := a1.Thumbnail(sum.ID, sum.CoverOrdinal, 192)
	if err != nil {
		t.Fatalf("thumb: %v", err)
	}
	a1.Shutdown(nil)

	// A fresh process must reuse the on-disk cache.
	a2, err := New(dataDir)
	if err != nil {
		t.Fatalf("new2: %v", err)
	}
	defer a2.Shutdown(nil)
	sum2, err := a2.IndexArchive(zipPath)
	if err != nil {
		t.Fatalf("index2: %v", err)
	}
	second, err := a2.Thumbnail(sum2.ID, sum2.CoverOrdinal, 192)
	if err != nil {
		t.Fatalf("thumb2: %v", err)
	}
	if !second.Cached {
		t.Error("thumbnail was regenerated after a restart")
	}
	if !bytes.Equal(first.Data, second.Data) {
		t.Error("cached bytes changed across a restart")
	}
}

func TestThumbnailInvalidatedWhenArchiveChanges(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "change.zip")
	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 200, 200, 10)}})

	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	first, err := a.Thumbnail(sum.ID, sum.CoverOrdinal, 192)
	if err != nil {
		t.Fatalf("thumb: %v", err)
	}

	// Replace the archive with different content of a different size.
	makeZip(t, p, []entry{
		{"a.jpg", encodeJPEG(t, 200, 200, 200)},
		{"b.jpg", encodeJPEG(t, 200, 200, 150)},
	})
	sum2, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index2: %v", err)
	}
	if sum2.ID != sum.ID {
		t.Fatalf("archive id changed: %d -> %d", sum.ID, sum2.ID)
	}
	second, err := a.Thumbnail(sum2.ID, sum2.CoverOrdinal, 192)
	if err != nil {
		t.Fatalf("thumb2: %v", err)
	}
	if second.Cached {
		t.Error("stale thumbnail was served after the archive changed")
	}
	if bytes.Equal(first.Data, second.Data) {
		t.Error("thumbnail bytes did not change with the archive")
	}
}

func TestFullImageReturnsOriginalBytes(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "full.zip")
	original := encodeJPEG(t, 320, 240, 77)
	makeZip(t, p, []entry{{"a.jpg", original}})

	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	resp, err := a.FullImage(sum.ID, sum.CoverOrdinal)
	if err != nil {
		t.Fatalf("full: %v", err)
	}
	if !bytes.Equal(resp.Data, original) {
		t.Error("full image bytes differ from the archived original")
	}
	if resp.ContentType != "image/jpeg" {
		t.Errorf("content type = %q", resp.ContentType)
	}
	if resp.Width != 320 || resp.Height != 240 {
		t.Errorf("size = %dx%d, want 320x240", resp.Width, resp.Height)
	}
}

func TestFullImageRejectsNonImagePayload(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "sneaky.zip")
	// An entry named .jpg whose bytes are HTML must never be served as an image.
	makeZip(t, p, []entry{{"evil.jpg", []byte("<html><script>alert(1)</script></html>")}})

	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if _, err := a.FullImage(sum.ID, sum.CoverOrdinal); err == nil {
		t.Fatal("expected an error for a non-image payload")
	}
}

func TestEmptyArchiveStatus(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "empty.zip")
	makeZip(t, p, []entry{{"notes.txt", []byte("no images")}})

	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if sum.Status != "empty" {
		t.Errorf("status = %q, want empty", sum.Status)
	}
	if sum.Count != 0 {
		t.Errorf("count = %d, want 0", sum.Count)
	}
	if sum.CoverOrdinal != -1 {
		t.Errorf("cover = %d, want -1", sum.CoverOrdinal)
	}
}

func TestCorruptArchiveStatus(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "broken.zip")
	if err := os.WriteFile(p, []byte("not a zip"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if sum.Status != "corrupt" {
		t.Errorf("status = %q, want corrupt", sum.Status)
	}
	if sum.Error == "" {
		t.Error("no error message recorded")
	}
}

func TestMissingArchiveStatus(t *testing.T) {
	a := testApp(t)
	sum, err := a.IndexArchive(filepath.Join(t.TempDir(), "gone.zip"))
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if sum.Status != "missing" {
		t.Errorf("status = %q, want missing", sum.Status)
	}
}

func TestPNGThumbnailKeepsAlpha(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "alpha.zip")
	makeZip(t, p, []entry{{"a.png", encodePNG(t, 200, 200)}})

	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	resp, err := a.Thumbnail(sum.ID, sum.CoverOrdinal, 128)
	if err != nil {
		t.Fatalf("thumb: %v", err)
	}
	if resp.ContentType != "image/png" {
		t.Errorf("content type = %q, want image/png", resp.ContentType)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	a := testApp(t)
	root := `C:\pictures`
	size := 256
	desc := true
	got, err := a.UpdateSettings(SettingsPatch{Root: &root, ThumbSize: &size, SortDesc: &desc})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Root != root || got.ThumbSize != size || !got.SortDesc {
		t.Errorf("settings = %+v", got)
	}
	// Out-of-range values are clamped rather than accepted.
	bad := 99999
	got, err = a.UpdateSettings(SettingsPatch{ThumbSize: &bad})
	if err != nil {
		t.Fatalf("update2: %v", err)
	}
	if got.ThumbSize == bad {
		t.Error("out-of-range thumbnail size was accepted")
	}
}

func TestCacheStatsAndClear(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "c.zip")
	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 200, 200, 30)}})
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if _, err := a.Thumbnail(sum.ID, sum.CoverOrdinal, 192); err != nil {
		t.Fatalf("thumb: %v", err)
	}

	stats, err := a.GetCacheStats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.ThumbCount == 0 {
		t.Error("no thumbnails counted")
	}
	if stats.ArchiveCount == 0 {
		t.Error("no archives counted")
	}

	if err := a.ClearCache(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	stats, err = a.GetCacheStats()
	if err != nil {
		t.Fatalf("stats2: %v", err)
	}
	if stats.ThumbCount != 0 || stats.ArchiveCount != 0 {
		t.Errorf("stats after clear = %+v", stats)
	}
}

func TestSortArchives(t *testing.T) {
	list := []ArchiveSummary{
		{Name: "b10.zip", Size: 10, ModTime: 100},
		{Name: "b2.zip", Size: 30, ModTime: 300},
		{Name: "a1.zip", Size: 20, ModTime: 200},
	}

	byName := append([]ArchiveSummary(nil), list...)
	SortArchives(byName, SortByName, false)
	if byName[0].Name != "a1.zip" || byName[1].Name != "b2.zip" || byName[2].Name != "b10.zip" {
		t.Errorf("name order = %v", names(byName))
	}

	bySize := append([]ArchiveSummary(nil), list...)
	SortArchives(bySize, SortBySize, false)
	if bySize[0].Size != 30 || bySize[2].Size != 10 {
		t.Errorf("size order = %v", names(bySize))
	}

	byTime := append([]ArchiveSummary(nil), list...)
	SortArchives(byTime, SortByMTime, false)
	if byTime[0].ModTime != 300 || byTime[2].ModTime != 100 {
		t.Errorf("mtime order = %v", names(byTime))
	}

	desc := append([]ArchiveSummary(nil), list...)
	SortArchives(desc, SortByName, true)
	if desc[0].Name != "b10.zip" {
		t.Errorf("desc order = %v", names(desc))
	}
}

func names(list []ArchiveSummary) []string {
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = a.Name
	}
	return out
}

func TestCheckEntryReportsBrokenImage(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "mixed.zip")
	makeZip(t, p, []entry{
		{"good.jpg", encodeJPEG(t, 100, 100, 40)},
		{"bad.jpg", []byte("garbage that is not an image")},
	})
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	detail, err := a.GetArchiveEntries(sum.ID)
	if err != nil {
		t.Fatalf("entries: %v", err)
	}
	var good, bad int
	for _, e := range detail.Entries {
		if e.Name == "good.jpg" {
			good = e.Ordinal
		} else {
			bad = e.Ordinal
		}
	}
	if st := a.CheckEntry(sum.ID, good); !st.OK {
		t.Errorf("good entry reported as broken: %+v", st)
	}
	if st := a.CheckEntry(sum.ID, bad); st.OK {
		t.Error("broken entry reported as ok")
	}
}

func TestUnknownArchiveID(t *testing.T) {
	a := testApp(t)
	if _, err := a.GetArchiveEntries(999999); err == nil {
		t.Fatal("expected an error for an unknown archive id")
	}
}
