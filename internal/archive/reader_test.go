package archive

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

// makeJPEG encodes a solid-colour JPEG of the given size.
func makeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

// makePNG encodes a transparent PNG of the given size.
func makePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{R: 10, G: 200, B: 90, A: uint8((x + y) % 256)})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

type zipEntry struct {
	name string
	data []byte
}

// writeZip builds a ZIP at path from the given entries.
func writeZip(t *testing.T, path string, entries []zipEntry) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatalf("create entry %q: %v", e.name, err)
		}
		if _, err := w.Write(e.data); err != nil {
			t.Fatalf("write entry %q: %v", e.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
}

func TestOpenListsOnlyImagesInNaturalOrder(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sample.zip")
	jpg := makeJPEG(t, 32, 24)
	writeZip(t, p, []zipEntry{
		{"page10.jpg", jpg},
		{"page2.jpg", jpg},
		{"notes.txt", []byte("ignore me")},
		{"page1.jpg", jpg},
		{"sub/page3.png", makePNG(t, 16, 16)},
		{"cover.html", []byte("<html></html>")},
	})

	a, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if got, want := a.Count(), 4; got != want {
		t.Fatalf("count = %d, want %d", got, want)
	}
	want := []string{"page1.jpg", "page2.jpg", "page10.jpg", "sub/page3.png"}
	for i, w := range want {
		if got := a.Entries()[i].Name; got != w {
			t.Errorf("entry %d = %q, want %q", i, got, w)
		}
	}
	// Non-image entries must never be listed, which is what keeps HTML out of
	// the viewer.
	for _, e := range a.Entries() {
		if !IsImageName(e.Name) {
			t.Errorf("non-image entry listed: %q", e.Name)
		}
	}
}

func TestOpenDoesNotModifyArchive(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "readonly.zip")
	writeZip(t, p, []zipEntry{{"a.jpg", makeJPEG(t, 8, 8)}})

	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	beforeInfo, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	a, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := a.ReadEntry(a.Entries()[0].Ordinal); err != nil {
		t.Fatalf("read entry: %v", err)
	}
	if _, err := a.DecodeImage(a.Entries()[0].Ordinal); err != nil {
		t.Fatalf("decode: %v", err)
	}

	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	afterInfo, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("archive bytes changed")
	}
	if beforeInfo.ModTime() != afterInfo.ModTime() || beforeInfo.Size() != afterInfo.Size() {
		t.Error("archive metadata changed")
	}
}

func TestDuplicateNamesUseOrdinal(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "dupes.zip")
	first := makeJPEG(t, 8, 8)
	second := makeJPEG(t, 16, 16)
	writeZip(t, p, []zipEntry{
		{"same.jpg", first},
		{"same.jpg", second},
	})

	a, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if a.Count() != 2 {
		t.Fatalf("count = %d, want 2", a.Count())
	}
	// Both entries keep distinct ordinals even though the names collide.
	if a.Entries()[0].Ordinal == a.Entries()[1].Ordinal {
		t.Fatal("duplicate names share an ordinal")
	}
	// The stable tie-break keeps the original entry order.
	if a.Entries()[0].Ordinal > a.Entries()[1].Ordinal {
		t.Error("duplicate names are not in original order")
	}
	// Reading by ordinal returns the matching payload.
	d0, err := a.DecodeImage(a.Entries()[0].Ordinal)
	if err != nil {
		t.Fatalf("decode first: %v", err)
	}
	d1, err := a.DecodeImage(a.Entries()[1].Ordinal)
	if err != nil {
		t.Fatalf("decode second: %v", err)
	}
	if d0.Info.Width == d1.Info.Width {
		t.Errorf("both duplicates decoded to the same image (width %d)", d0.Info.Width)
	}
}

func TestEntryByteLimit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big.zip")
	writeZip(t, p, []zipEntry{{"big.jpg", makeJPEG(t, 64, 64)}})

	// A limit below the entry size must reject it without reading the payload.
	limits := Limits{MaxEntryBytes: 64, MaxPixels: DefaultLimits.MaxPixels, MaxEntries: 100}
	a, err := Open(p, limits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_, err = a.ReadEntry(a.Entries()[0].Ordinal)
	if err == nil {
		t.Fatal("expected size limit error")
	}
	if !isErr(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestPixelLimit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "huge.zip")
	writeZip(t, p, []zipEntry{{"huge.png", makePNG(t, 200, 200)}})

	limits := Limits{MaxEntryBytes: DefaultLimits.MaxEntryBytes, MaxPixels: 1000, MaxEntries: 100}
	a, err := Open(p, limits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_, err = a.DecodeImage(a.Entries()[0].Ordinal)
	if err == nil {
		t.Fatal("expected pixel limit error")
	}
	if !isErr(err, ErrTooManyPixels) {
		t.Fatalf("err = %v, want ErrTooManyPixels", err)
	}
}

func TestEntryCountLimit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "many.zip")
	jpg := makeJPEG(t, 4, 4)
	var entries []zipEntry
	for i := 0; i < 20; i++ {
		entries = append(entries, zipEntry{name: string(rune('a'+i)) + ".jpg", data: jpg})
	}
	writeZip(t, p, entries)

	limits := Limits{MaxEntryBytes: DefaultLimits.MaxEntryBytes, MaxPixels: DefaultLimits.MaxPixels, MaxEntries: 5}
	a, err := Open(p, limits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if a.Count() != 5 {
		t.Fatalf("count = %d, want 5", a.Count())
	}
	if a.Skipped() == 0 {
		t.Error("skipped count not reported")
	}
}

func TestCorruptArchive(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "broken.zip")
	if err := os.WriteFile(p, []byte("this is not a zip file at all"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := Open(p, DefaultLimits)
	if err == nil {
		t.Fatal("expected error for corrupt archive")
	}
	if !isErr(err, ErrCorrupt) {
		t.Fatalf("err = %v, want ErrCorrupt", err)
	}
}

func TestTruncatedEntry(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "truncated.zip")
	writeZip(t, p, []zipEntry{{"a.jpg", makeJPEG(t, 64, 64)}})

	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// Cut the file in the middle of the entry payload, keeping the central
	// directory intact so the archive still opens.
	cut := len(data) / 2
	if err := os.WriteFile(p, data[:cut], 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	a, err := Open(p, DefaultLimits)
	if err != nil {
		// A truncated central directory is also an acceptable outcome.
		if !isErr(err, ErrCorrupt) {
			t.Fatalf("open err = %v, want ErrCorrupt", err)
		}
		return
	}
	if a.Count() == 0 {
		return
	}
	if _, err := a.ReadEntry(a.Entries()[0].Ordinal); err == nil {
		t.Fatal("expected error reading truncated entry")
	}
}

// patchZipField rewrites a 2-byte little-endian field in every local file
// header and central directory header that carries the given signature. The
// offset is relative to the start of the signature.
func patchZipField(t *testing.T, path string, sig []byte, offset int, value uint16) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	patched := 0
	for i := 0; i+4 <= len(data); i++ {
		if !bytes.Equal(data[i:i+4], sig) {
			continue
		}
		at := i + offset
		if at+2 > len(data) {
			continue
		}
		data[at] = byte(value)
		data[at+1] = byte(value >> 8)
		patched++
	}
	if patched == 0 {
		t.Fatalf("no %x header found in %s", sig, path)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

var (
	localHeaderSig = []byte{'P', 'K', 3, 4}
	centralDirSig  = []byte{'P', 'K', 1, 2}
)

// Field offsets inside the two header layouts.
const (
	localFlagsOffset   = 6
	localMethodOffset  = 8
	centralFlagsOffset = 8
	centralMethodOffset = 10
)

func TestEncryptedEntryIsListedButNotDecoded(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "enc.zip")
	writeZip(t, p, []zipEntry{{"secret.jpg", makeJPEG(t, 8, 8)}})

	// Set the encryption bit in both headers so the reader treats the entry as
	// password protected.
	patchZipField(t, p, localHeaderSig, localFlagsOffset, 0x0001)
	patchZipField(t, p, centralDirSig, centralFlagsOffset, 0x0001)

	a, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if a.Count() != 1 {
		t.Fatalf("count = %d, want 1", a.Count())
	}
	if !a.Entries()[0].Encrypted {
		t.Fatal("entry not marked encrypted")
	}
	if _, err := a.ReadEntry(a.Entries()[0].Ordinal); !isErr(err, ErrEncrypted) {
		t.Fatalf("err = %v, want ErrEncrypted", err)
	}
}

func TestEmptyArchive(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "empty.zip")
	writeZip(t, p, nil)

	a, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if a.Count() != 0 {
		t.Fatalf("count = %d, want 0", a.Count())
	}
}

func TestFingerprintChangesWithContent(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "fp.zip")
	writeZip(t, p, []zipEntry{{"a.jpg", makeJPEG(t, 8, 8)}})
	a1, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	writeZip(t, p, []zipEntry{{"a.jpg", makeJPEG(t, 8, 8)}, {"b.jpg", makeJPEG(t, 8, 8)}})
	a2, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if a1.Fingerprint() == a2.Fingerprint() {
		t.Error("fingerprint did not change after the archive changed")
	}
}

func TestFingerprintStableForSameContent(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "stable.zip")
	writeZip(t, p, []zipEntry{{"a.jpg", makeJPEG(t, 8, 8)}})
	a1, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	a2, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if a1.Fingerprint() != a2.Fingerprint() {
		t.Error("fingerprint is not stable for unchanged content")
	}
}

func TestDecodeConfigMatchesDecode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cfg.zip")
	writeZip(t, p, []zipEntry{{"a.png", makePNG(t, 40, 30)}})
	a, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ord := a.Entries()[0].Ordinal
	info, err := a.DecodeConfig(ord)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	decoded, err := a.DecodeImage(ord)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if info.Width != decoded.Info.Width || info.Height != decoded.Info.Height {
		t.Errorf("config %dx%d != decoded %dx%d", info.Width, info.Height, decoded.Info.Width, decoded.Info.Height)
	}
	if info.Format != "png" {
		t.Errorf("format = %q, want png", info.Format)
	}
}

func TestUnsupportedCompressionMethod(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bz.zip")
	writeZip(t, p, []zipEntry{{"a.jpg", makeJPEG(t, 8, 8)}})

	// Rewrite the method field to BZIP2 (12) in both headers.
	patchZipField(t, p, localHeaderSig, localMethodOffset, 12)
	patchZipField(t, p, centralDirSig, centralMethodOffset, 12)

	a, err := Open(p, DefaultLimits)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if a.Count() != 1 {
		t.Fatalf("count = %d, want 1", a.Count())
	}
	if _, err := a.ReadEntry(a.Entries()[0].Ordinal); !isErr(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

// isErr reports whether err matches target through the EntryError wrapper.
func isErr(err, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
