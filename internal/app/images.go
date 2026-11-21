package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tonbiattack/zip-scope/internal/archive"
	"github.com/tonbiattack/zip-scope/internal/imgserve"
	"github.com/tonbiattack/zip-scope/internal/prefetch"
	"github.com/tonbiattack/zip-scope/internal/store"
	"github.com/tonbiattack/zip-scope/internal/thumb"
)

// ImageResponse is one served image.
type ImageResponse struct {
	// Data is the encoded image bytes.
	Data []byte
	// ContentType is the MIME type of Data.
	ContentType string
	// Cached reports whether the bytes came from the disk cache.
	Cached bool
	// Width and Height are the pixel dimensions of Data.
	Width  int
	Height int
}

// ImageError is returned when an image cannot be produced. Kind lets the UI
// distinguish "this entry is broken" from "this entry is too big".
type ImageError struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

func (e *ImageError) Error() string { return e.Message }

// Error kinds reported to the UI.
const (
	ErrKindEncrypted   = "encrypted"
	ErrKindCorrupt     = "corrupt"
	ErrKindUnsupported = "unsupported"
	ErrKindTooLarge    = "too-large"
	ErrKindTooManyPix  = "too-many-pixels"
	ErrKindMissing     = "missing"
	ErrKindUnknown     = "unknown"
)

func classifyImageError(err error) *ImageError {
	if err == nil {
		return nil
	}
	var ie *ImageError
	if errors.As(err, &ie) {
		return ie
	}
	kind := ErrKindUnknown
	switch {
	case errors.Is(err, archive.ErrEncrypted):
		kind = ErrKindEncrypted
	case errors.Is(err, archive.ErrChecksum), errors.Is(err, archive.ErrCorrupt),
		errors.Is(err, archive.ErrTruncated), errors.Is(err, archive.ErrNotImage):
		kind = ErrKindCorrupt
	case errors.Is(err, archive.ErrUnsupported):
		kind = ErrKindUnsupported
	case errors.Is(err, archive.ErrTooLarge):
		kind = ErrKindTooLarge
	case errors.Is(err, archive.ErrTooManyPixels):
		kind = ErrKindTooManyPix
	case errors.Is(err, os.ErrNotExist):
		kind = ErrKindMissing
	}
	return &ImageError{Kind: kind, Message: err.Error()}
}

// thumbKey builds the cache key for one entry thumbnail.
func (a *App) thumbKey(arc *archive.Archive, ordinal, width int) (thumb.Key, bool) {
	idx := arc.IndexOfOrdinal(ordinal)
	if idx < 0 {
		return thumb.Key{}, false
	}
	e, _ := arc.Entry(idx)
	return thumb.Key{
		Archive: arc.Fingerprint(),
		Ordinal: e.Ordinal,
		CRC32:   e.CRC32,
		Size:    e.UncompressedSize,
		Width:   width,
		Version: thumb.Version,
	}, true
}

// Thumbnail returns a cached-or-generated thumbnail for one entry. It is the
// synchronous path used by the HTTP handler.
//
// Concurrent requests for the same key are collapsed into one generation, and
// the result is validated before it is served: a cache file that no longer
// decodes as an image is treated as a miss and regenerated.
func (a *App) Thumbnail(id int64, ordinal, width int) (*ImageResponse, error) {
	oa, err := a.lookup(id)
	if err != nil {
		return nil, classifyImageError(err)
	}
	width = snapWidth(width)
	key, ok := a.thumbKey(oa.arc, ordinal, width)
	if !ok {
		return nil, &ImageError{Kind: ErrKindMissing, Message: "entry not found"}
	}
	keyStr := key.String()

	// Memory cache first: a thumbnail that was just generated for the grid is
	// served without touching the disk.
	if v, ok := a.mem.Get("t:" + keyStr); ok {
		if r, ok := v.(*ImageResponse); ok {
			// Report the hit honestly even though the bytes never left memory.
			hit := *r
			hit.Cached = true
			return &hit, nil
		}
	}

	// Collapse concurrent requests for the same thumbnail.
	v, err := a.flight.Do("t:"+keyStr, func() (any, error) {
		return a.generateThumbnail(oa.arc, key, keyStr, ordinal, width)
	})
	if err != nil {
		return nil, err
	}
	resp, ok := v.(*ImageResponse)
	if !ok || resp == nil {
		return nil, &ImageError{Kind: ErrKindUnknown, Message: "thumbnail generation failed"}
	}
	return resp, nil
}

// generateThumbnail produces one thumbnail, consulting the disk cache first.
func (a *App) generateThumbnail(arc *archive.Archive, key thumb.Key, keyStr string, ordinal, width int) (*ImageResponse, error) {
	if data, ok := a.st.LookupThumb(keyStr); ok {
		// A cache file can be truncated by a crash or an external tool. Only
		// serve bytes that still decode as an image.
		if ct, w, h, ok := validateThumb(data); ok {
			resp := &ImageResponse{Data: data, ContentType: ct, Cached: true, Width: w, Height: h}
			a.mem.Put("t:"+keyStr, int64(len(data)), resp)
			return resp, nil
		}
		// Drop the unusable entry so the next request regenerates cleanly.
		a.st.ForgetThumb(keyStr)
	}

	decoded, err := a.decodeEntry(arc, ordinal)
	if err != nil {
		return nil, classifyImageError(err)
	}
	res, err := thumb.Generate(decoded.Image, width)
	if err != nil {
		return nil, classifyImageError(err)
	}
	ext := ".jpg"
	if res.Format == "png" {
		ext = ".png"
	}
	if _, err := a.st.PutThumb(keyStr, key.Shard(), ext, res.Data); err != nil {
		// A cache write failure must not break viewing.
		fmt.Fprintf(os.Stderr, "zipscope: cache write failed: %v\n", err)
	}
	go a.st.Evict(false)

	resp := &ImageResponse{
		Data:        res.Data,
		ContentType: imgserve.ContentType(res.Format),
		Width:       res.Width,
		Height:      res.Height,
	}
	a.mem.Put("t:"+keyStr, int64(len(res.Data)), resp)
	return resp, nil
}

// validateThumb reports whether data is a decodable image and returns its
// content type and dimensions.
func validateThumb(data []byte) (string, int, int, bool) {
	if len(data) == 0 {
		return "", 0, 0, false
	}
	ct := imgserve.SniffContentType(data)
	if ct == "" {
		return "", 0, 0, false
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return "", 0, 0, false
	}
	return ct, cfg.Width, cfg.Height, true
}

// FullImage returns the original encoded bytes of one entry, which is what the
// full-size viewer displays. The bytes are the archive's own data, so no
// re-encoding loss is introduced.
func (a *App) FullImage(id int64, ordinal int) (*ImageResponse, error) {
	oa, err := a.lookup(id)
	if err != nil {
		return nil, classifyImageError(err)
	}
	cacheKey := fmt.Sprintf("f:%d:%d", id, ordinal)
	if v, ok := a.mem.Get(cacheKey); ok {
		if r, ok := v.(*ImageResponse); ok {
			return r, nil
		}
	}
	data, err := oa.arc.ReadEntry(ordinal)
	if err != nil {
		return nil, classifyImageError(err)
	}
	// Validate the header before handing the bytes to the WebView so an
	// oversized or non-image payload never reaches the renderer.
	info, err := a.decodeConfig(oa.arc, ordinal, data)
	if err != nil {
		return nil, classifyImageError(err)
	}
	ct := imgserve.SniffContentType(data)
	if ct == "" {
		return nil, &ImageError{Kind: ErrKindUnsupported, Message: "unsupported image container"}
	}
	resp := &ImageResponse{Data: data, ContentType: ct, Width: info.Width, Height: info.Height}
	a.mem.Put(cacheKey, int64(len(data)), resp)
	return resp, nil
}

// decodeEntry decodes one entry while charging the decode working set against
// the shared budget, so concurrent decodes cannot exhaust memory.
func (a *App) decodeEntry(arc *archive.Archive, ordinal int) (*archive.DecodedImage, error) {
	idx := arc.IndexOfOrdinal(ordinal)
	if idx < 0 {
		return nil, &ImageError{Kind: ErrKindMissing, Message: "entry not found"}
	}
	e, _ := arc.Entry(idx)

	// Estimate the working set: the compressed bytes plus the decoded RGBA
	// buffer. The pixel limit already bounds the second term.
	est := int64(e.UncompressedSize) + int64(e.UncompressedSize)/2
	if est < 1<<20 {
		est = 1 << 20
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := a.work.Acquire(ctx, est); err != nil {
		return nil, err
	}
	defer a.work.Release(est)

	return arc.DecodeImage(ordinal)
}

// decodeConfig reads only the header, still charging the working set.
func (a *App) decodeConfig(arc *archive.Archive, ordinal int, data []byte) (archive.ImageInfo, error) {
	est := int64(len(data))
	if est < 1<<20 {
		est = 1 << 20
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := a.work.Acquire(ctx, est); err != nil {
		return archive.ImageInfo{}, err
	}
	defer a.work.Release(est)
	return arc.DecodeConfigFromBytes(data)
}

// PrefetchImage warms the cache for one entry at the given priority. It is
// fire-and-forget: the result is only stored, never returned.
func (a *App) PrefetchImage(id int64, ordinal, width int, priority int) {
	width = snapWidth(width)
	key := fmt.Sprintf("p:%d:%d:%d", id, ordinal, width)
	a.sched.Submit(&prefetch.Job{
		Key:      key,
		Priority: priority,
		Run: func(ctx context.Context) (any, error) {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			_, err := a.Thumbnail(id, ordinal, width)
			return nil, err
		},
	})
}

// PrefetchFull warms the full-size cache for one entry.
func (a *App) PrefetchFull(id int64, ordinal int, priority int) {
	key := fmt.Sprintf("pf:%d:%d", id, ordinal)
	a.sched.Submit(&prefetch.Job{
		Key:      key,
		Priority: priority,
		Run: func(ctx context.Context) (any, error) {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			_, err := a.FullImage(id, ordinal)
			return nil, err
		},
	})
}

// BumpSelection tells the prefetcher that the selection changed, discarding
// speculative work for the previous one.
func (a *App) BumpSelection() uint64 {
	return a.sched.BumpEpoch()
}

// PrefetchStats reports scheduler and memory counters.
type PrefetchStats struct {
	Scheduler prefetch.Stats     `json:"scheduler"`
	Memory    prefetch.CacheStats `json:"memory"`
	Decode    int64              `json:"decodeBytes"`
}

// GetPrefetchStats returns the prefetch counters for diagnostics.
func (a *App) GetPrefetchStats() PrefetchStats {
	return PrefetchStats{
		Scheduler: a.sched.Stats(),
		Memory:    a.mem.Stats(),
		Decode:    a.work.Used(),
	}
}

// EntryStatus reports whether one entry can be displayed, without decoding it.
type EntryStatus struct {
	Ordinal int    `json:"ordinal"`
	OK      bool   `json:"ok"`
	Kind    string `json:"kind,omitempty"`
	Message string `json:"message,omitempty"`
}

// CheckEntry validates one entry's header and reports its status.
func (a *App) CheckEntry(id int64, ordinal int) EntryStatus {
	oa, err := a.lookup(id)
	if err != nil {
		ie := classifyImageError(err)
		return EntryStatus{Ordinal: ordinal, Kind: ie.Kind, Message: ie.Message}
	}
	if _, err := a.decodeEntry(oa.arc, ordinal); err != nil {
		ie := classifyImageError(err)
		return EntryStatus{Ordinal: ordinal, Kind: ie.Kind, Message: ie.Message}
	}
	return EntryStatus{Ordinal: ordinal, OK: true}
}

// WarmArchive generates thumbnails for the first n entries of an archive at low
// priority, which makes the grid feel instant on a second visit.
func (a *App) WarmArchive(id int64, n, width int) {
	oa, err := a.lookup(id)
	if err != nil {
		return
	}
	width = snapWidth(width)
	entries := oa.arc.Entries()
	if n <= 0 || n > len(entries) {
		n = len(entries)
	}
	for i := 0; i < n; i++ {
		a.PrefetchImage(id, entries[i].Ordinal, width, prefetch.PriorityVisible)
	}
}

// ArchiveCover returns the representative thumbnail for an archive id.
func (a *App) ArchiveCover(id int64, width int) (*ImageResponse, error) {
	oa, err := a.lookup(id)
	if err != nil {
		return nil, classifyImageError(err)
	}
	if oa.arc.Count() == 0 {
		return nil, &ImageError{Kind: ErrKindMissing, Message: "archive has no images"}
	}
	return a.Thumbnail(id, oa.arc.Entries()[0].Ordinal, width)
}

// ResolveImageID is used by the HTTP handler to turn a request path into a
// validated identifier.
func ResolveImageID(raw string) (imgserve.ID, error) {
	return imgserve.Parse(strings.TrimPrefix(raw, "/"))
}

// storeStatusFor maps an archive error to a stored status string.
func storeStatusFor(err error) string {
	switch {
	case err == nil:
		return store.StatusOK
	case errors.Is(err, archive.ErrEncrypted):
		return store.StatusEncrypted
	case errors.Is(err, archive.ErrCorrupt):
		return store.StatusCorrupt
	case errors.Is(err, os.ErrNotExist):
		return store.StatusMissing
	default:
		return store.StatusError
	}
}

// touch keeps the open-archive map from growing without bound.
func (a *App) touch() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.archives) <= 64 {
		return
	}
	cutoff := time.Now().Add(-10 * time.Minute)
	for id, oa := range a.archives {
		if oa.lastUse.Before(cutoff) {
			delete(a.archives, id)
		}
	}
}

// imageSize is a small helper for tests.
func imageSize(img image.Image) (int, int) {
	b := img.Bounds()
	return b.Dx(), b.Dy()
}

// cachePathFor exposes the on-disk location of a thumbnail, for diagnostics.
func (a *App) cachePathFor(key thumb.Key, ext string) string {
	p, err := a.st.ThumbPath(key.String(), key.Shard(), ext)
	if err != nil {
		return ""
	}
	return filepath.Clean(p)
}
