package archive

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	// Register the still-image decoders used by the viewer. WebP is not in the
	// standard library, so it is pulled in explicitly.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

// Limits bounds the work a single archive may cause. Zero values fall back to
// DefaultLimits, and no limit may be disabled: the spec requires that the
// viewer never becomes unbounded.
type Limits struct {
	// MaxEntryBytes caps the uncompressed size of one image entry.
	MaxEntryBytes int64
	// MaxPixels caps width*height of one decoded image.
	MaxPixels int64
	// MaxEntries caps the number of entries in one archive.
	MaxEntries int
}

// DefaultLimits are the MVP initial values from the specification.
var DefaultLimits = Limits{
	MaxEntryBytes: 128 << 20, // 128 MiB per image
	MaxPixels:     40_000_000,
	MaxEntries:    100_000,
}

func (l Limits) normalized() Limits {
	if l.MaxEntryBytes <= 0 {
		l.MaxEntryBytes = DefaultLimits.MaxEntryBytes
	}
	if l.MaxPixels <= 0 {
		l.MaxPixels = DefaultLimits.MaxPixels
	}
	if l.MaxEntries <= 0 {
		l.MaxEntries = DefaultLimits.MaxEntries
	}
	return l
}

// imageExts is the allowlist of still-image extensions. Anything else is never
// decoded, which keeps HTML/SVG and other active content out of the viewer.
var imageExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".jpe":  true,
	".jfif": true,
	".png":  true,
	".gif":  true,
	".webp": true,
}

// IsImageName reports whether name has an extension the viewer will decode.
func IsImageName(name string) bool {
	return imageExts[strings.ToLower(path.Ext(name))]
}

// Entry describes one image entry inside an archive.
type Entry struct {
	// Ordinal is the entry's position in the archive's central directory. It
	// disambiguates duplicate names.
	Ordinal int `json:"ordinal"`
	// Name is the raw entry name as stored in the archive.
	Name string `json:"name"`
	// CRC32 is the entry checksum, part of the cache key.
	CRC32 uint32 `json:"crc32"`
	// CompressedSize and UncompressedSize are the stored sizes.
	CompressedSize   uint64 `json:"compressedSize"`
	UncompressedSize uint64 `json:"uncompressedSize"`
	// Method is the ZIP compression method (0 store, 8 deflate).
	Method uint16 `json:"method"`
	// Encrypted marks password protected entries, which are listed but never
	// decoded.
	Encrypted bool `json:"encrypted"`
}

// Archive is an opened ZIP/CBZ with its image entries indexed.
type Archive struct {
	path        string
	size        int64
	mtime       time.Time
	fingerprint string
	entries     []Entry
	limits      Limits
	// skipped counts entries that were rejected by the entry limit.
	skipped int
}

// Path returns the archive's filesystem path.
func (a *Archive) Path() string { return a.path }

// Size returns the archive file size in bytes.
func (a *Archive) Size() int64 { return a.size }

// ModTime returns the archive file modification time.
func (a *Archive) ModTime() time.Time { return a.mtime }

// Fingerprint identifies the archive's content listing. It is recomputed on
// every open and compared against the cached value to detect changes.
func (a *Archive) Fingerprint() string { return a.fingerprint }

// Entries returns the image entries in natural order.
func (a *Archive) Entries() []Entry { return a.entries }

// Count returns the number of image entries.
func (a *Archive) Count() int { return len(a.entries) }

// Skipped returns how many entries were dropped because of the entry limit.
func (a *Archive) Skipped() int { return a.skipped }

// Limits returns the limits this archive was opened with.
func (a *Archive) Limits() Limits { return a.limits }

// Open indexes the image entries of the archive at p. It reads only the central
// directory: no entry payload is decompressed, and the file is opened read-only.
func Open(p string, limits Limits) (*Archive, error) {
	limits = limits.normalized()

	fi, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("%s: %w", p, ErrCorrupt)
	}

	zr, err := zip.OpenReader(p)
	if err != nil {
		return nil, classifyOpenError(p, err)
	}
	defer zr.Close()

	a := &Archive{
		path:   p,
		size:   fi.Size(),
		mtime:  fi.ModTime(),
		limits: limits,
	}

	// Collect image entries. The ordinal is the central-directory index, so it
	// stays stable for a given archive revision.
	for i, f := range zr.File {
		if f.FileInfo().IsDir() || strings.HasSuffix(f.Name, "/") {
			continue
		}
		if !IsImageName(f.Name) {
			continue
		}
		if len(a.entries) >= limits.MaxEntries {
			a.skipped = len(zr.File) - i
			break
		}
		a.entries = append(a.entries, Entry{
			Ordinal:          i,
			Name:             f.Name,
			CRC32:            f.CRC32,
			CompressedSize:   f.CompressedSize64,
			UncompressedSize: f.UncompressedSize64,
			Method:           f.Method,
			Encrypted:        f.Flags&0x1 != 0,
		})
	}

	// Natural order over the full path, with the original entry order as a
	// stable tie-break for equal names.
	sort.SliceStable(a.entries, func(i, j int) bool {
		if c := NaturalCompare(a.entries[i].Name, a.entries[j].Name); c != 0 {
			return c < 0
		}
		return a.entries[i].Ordinal < a.entries[j].Ordinal
	})

	a.fingerprint = fingerprint(a.entries, a.skipped)
	return a, nil
}

func classifyOpenError(p string, err error) error {
	switch {
	case errors.Is(err, zip.ErrFormat):
		return fmt.Errorf("%s: %w", p, ErrCorrupt)
	case errors.Is(err, os.ErrNotExist), errors.Is(err, os.ErrPermission):
		return err
	default:
		return fmt.Errorf("%s: %w: %v", p, ErrCorrupt, err)
	}
}

// fingerprint hashes the entry listing so that a changed archive is detected
// even when its size and mtime happen to match.
func fingerprint(entries []Entry, skipped int) string {
	h := sha256.New()
	var buf [8]byte
	for _, e := range entries {
		binary.LittleEndian.PutUint32(buf[:4], uint32(len(e.Name)))
		h.Write(buf[:4])
		h.Write([]byte(e.Name))
		binary.LittleEndian.PutUint32(buf[:4], e.CRC32)
		h.Write(buf[:4])
		binary.LittleEndian.PutUint64(buf[:8], e.UncompressedSize)
		h.Write(buf[:8])
		binary.LittleEndian.PutUint64(buf[:8], e.CompressedSize)
		h.Write(buf[:8])
	}
	binary.LittleEndian.PutUint64(buf[:8], uint64(skipped))
	h.Write(buf[:8])
	return hex.EncodeToString(h.Sum(nil)[:16])
}

// Entry returns the entry at the given natural-order index.
func (a *Archive) Entry(index int) (Entry, bool) {
	if index < 0 || index >= len(a.entries) {
		return Entry{}, false
	}
	return a.entries[index], true
}

// IndexOfOrdinal maps a central-directory ordinal back to its natural-order
// index, which is what the UI uses.
func (a *Archive) IndexOfOrdinal(ordinal int) int {
	for i := range a.entries {
		if a.entries[i].Ordinal == ordinal {
			return i
		}
	}
	return -1
}

// ReadEntry streams one entry and returns its uncompressed bytes. The byte
// limit is enforced both from the header and while reading, so a lying header
// cannot exhaust memory.
//
// The archive is reopened for every read, so the file may have changed since it
// was indexed. Before returning anything, the entry at ordinal is checked
// against the identity recorded at index time: if the name, CRC or size differ,
// the archive was replaced and the caller is told with ErrArchiveChanged rather
// than being handed a different image.
func (a *Archive) ReadEntry(ordinal int) ([]byte, error) {
	zr, err := zip.OpenReader(a.path)
	if err != nil {
		return nil, classifyOpenError(a.path, err)
	}
	defer zr.Close()

	if ordinal < 0 || ordinal >= len(zr.File) {
		return nil, entryErr("", ErrArchiveChanged)
	}

	// The ordinal must still point at the entry that was indexed. A replaced
	// archive can have the same entry count with different contents, so the
	// identity is compared rather than the position alone.
	want, ok := a.entryByOrdinal(ordinal)
	if !ok {
		return nil, entryErr("", ErrArchiveChanged)
	}
	f := zr.File[ordinal]
	if f.Name != want.Name || f.CRC32 != want.CRC32 ||
		f.UncompressedSize64 != want.UncompressedSize {
		return nil, entryErr(want.Name, ErrArchiveChanged)
	}

	return a.readFile(f)
}

// entryByOrdinal returns the indexed entry with the given central-directory
// ordinal.
func (a *Archive) entryByOrdinal(ordinal int) (Entry, bool) {
	for i := range a.entries {
		if a.entries[i].Ordinal == ordinal {
			return a.entries[i], true
		}
	}
	return Entry{}, false
}

// Changed reports whether the archive on disk no longer matches the index this
// handle was built from. It is cheap enough to call before a batch of reads:
// the size and modification time are compared first, and only a mismatch in
// those triggers a full re-index comparison.
func (a *Archive) Changed() bool {
	fi, err := os.Stat(a.path)
	if err != nil {
		// A missing file is a change: the archive was moved or the drive was
		// unplugged.
		return true
	}
	if fi.Size() != a.size || !fi.ModTime().Equal(a.mtime) {
		return true
	}
	// Size and mtime can be preserved by a tool that rewrites the file, so the
	// central directory is compared as well.
	fresh, err := Open(a.path, a.limits)
	if err != nil {
		return true
	}
	return fresh.Fingerprint() != a.fingerprint
}

func (a *Archive) readFile(f *zip.File) ([]byte, error) {
	if f.Flags&0x1 != 0 {
		return nil, entryErr(f.Name, ErrEncrypted)
	}
	if f.Method != zip.Store && f.Method != zip.Deflate {
		return nil, entryErr(f.Name, ErrUnsupported)
	}
	if f.UncompressedSize64 > uint64(a.limits.MaxEntryBytes) {
		return nil, entryErr(f.Name, ErrTooLarge)
	}

	rc, err := f.Open()
	if err != nil {
		return nil, entryErr(f.Name, classifyReadError(err))
	}
	defer rc.Close()

	// Read at most one byte past the limit so an oversized entry is detected
	// without buffering all of it.
	limit := a.limits.MaxEntryBytes
	buf, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, entryErr(f.Name, classifyReadError(err))
	}
	if int64(len(buf)) > limit {
		return nil, entryErr(f.Name, ErrTooLarge)
	}
	// archive/zip verifies the CRC when the reader reaches EOF; ReadAll does
	// reach EOF, so a mismatch surfaces here.
	if err := rc.Close(); err != nil {
		return nil, entryErr(f.Name, classifyReadError(err))
	}
	return buf, nil
}

func classifyReadError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, zip.ErrChecksum):
		return ErrChecksum
	case errors.Is(err, zip.ErrFormat):
		return ErrCorrupt
	case errors.Is(err, zip.ErrAlgorithm):
		return ErrUnsupported
	case errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		return ErrTruncated
	case errors.Is(err, os.ErrPermission):
		return err
	default:
		return fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
}

// ImageInfo is the decoded header of one entry: enough to lay out the UI
// without paying for a full decode.
type ImageInfo struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Format string `json:"format"`
}

// DecodeConfig reads only the image header of an entry and enforces the pixel
// limit before any full decode happens.
func (a *Archive) DecodeConfig(ordinal int) (ImageInfo, error) {
	data, err := a.ReadEntry(ordinal)
	if err != nil {
		return ImageInfo{}, err
	}
	return a.configFromBytes(data)
}

// DecodeConfigFromBytes inspects an already-read entry payload. It exists so
// callers that must validate bytes before forwarding them can reuse the same
// pixel-limit check.
func (a *Archive) DecodeConfigFromBytes(data []byte) (ImageInfo, error) {
	return a.configFromBytes(data)
}

func (a *Archive) configFromBytes(data []byte) (ImageInfo, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return ImageInfo{}, entryErr("", classifyDecodeError(err))
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return ImageInfo{}, entryErr("", ErrCorrupt)
	}
	if int64(cfg.Width)*int64(cfg.Height) > a.limits.MaxPixels {
		return ImageInfo{}, entryErr("", ErrTooManyPixels)
	}
	return ImageInfo{Width: cfg.Width, Height: cfg.Height, Format: format}, nil
}

func classifyDecodeError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, image.ErrFormat):
		return ErrNotImage
	case errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		return ErrTruncated
	default:
		return fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
}

// DecodedImage is a fully decoded still image plus its header information.
type DecodedImage struct {
	Image  image.Image
	Info   ImageInfo
	Bytes  int
	Source []byte
}

// DecodeImage reads and decodes one entry. The header is inspected first so an
// image over the pixel limit is rejected before the expensive decode.
func (a *Archive) DecodeImage(ordinal int) (*DecodedImage, error) {
	data, err := a.ReadEntry(ordinal)
	if err != nil {
		return nil, err
	}
	info, err := a.configFromBytes(data)
	if err != nil {
		return nil, err
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, entryErr("", classifyDecodeError(err))
	}
	info.Format = format
	return &DecodedImage{Image: img, Info: info, Bytes: len(data), Source: data}, nil
}
