// Package thumb turns decoded images into small cached thumbnails. Keys are
// derived from the archive identity, the entry identity, the requested size and
// a generation version, so a changed archive or a changed encoder can never
// serve a stale image.
package thumb

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"strconv"

	xdraw "golang.org/x/image/draw"
)

// Version is bumped whenever the resize or encode pipeline changes, which
// invalidates every previously generated thumbnail.
const Version = 1

// sizeLadder is the set of thumbnail widths the UI can ask for. Requests are
// snapped to the nearest rung so that dragging the grid-size slider reuses
// cached images instead of generating one per pixel width.
var sizeLadder = []int{96, 128, 192, 256, 384, 512, 768, 1024}

// SnapSize maps an arbitrary requested width onto the supported ladder.
func SnapSize(w int) int {
	if w <= sizeLadder[0] {
		return sizeLadder[0]
	}
	for _, s := range sizeLadder {
		if w <= s {
			return s
		}
	}
	return sizeLadder[len(sizeLadder)-1]
}

// Key identifies one cached thumbnail.
type Key struct {
	// Archive is the archive fingerprint, so a modified archive misses.
	Archive string
	// Ordinal is the entry's central-directory index.
	Ordinal int
	// CRC32 and Size are the entry identity, guarding against an archive whose
	// fingerprint collides.
	CRC32 uint32
	Size  uint64
	// Width is the snapped thumbnail width.
	Width int
	// Version is the encoder generation.
	Version int
}

// String renders the key as a filesystem-safe, fixed-length name.
func (k Key) String() string {
	h := sha256.New()
	fmt.Fprintf(h, "v%d|%s|%d|%08x|%d|%d", k.Version, k.Archive, k.Ordinal, k.CRC32, k.Size, k.Width)
	return hex.EncodeToString(h.Sum(nil)[:20])
}

// Shard returns a two-level directory prefix so a single directory never holds
// hundreds of thousands of files.
func (k Key) Shard() string {
	s := k.String()
	return s[0:2] + "/" + s[2:4]
}

// Result is an encoded thumbnail.
type Result struct {
	Data   []byte
	Width  int
	Height int
	Format string
}

// Generate scales img so its longest side is at most maxSide and encodes the
// result. Images already smaller than the target are encoded as-is rather than
// upscaled.
func Generate(img image.Image, maxSide int) (*Result, error) {
	if maxSide <= 0 {
		maxSide = 256
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return nil, fmt.Errorf("thumb: empty image")
	}

	w, h := b.Dx(), b.Dy()
	if w > maxSide || h > maxSide {
		if w >= h {
			h = max(1, h*maxSide/w)
			w = maxSide
		} else {
			w = max(1, w*maxSide/h)
			h = maxSide
		}
	}

	var scaled image.Image
	if w == b.Dx() && h == b.Dy() {
		scaled = img
	} else {
		dst := image.NewNRGBA(image.Rect(0, 0, w, h))
		// CatmullRom is a good quality/speed trade-off for downscaling photos.
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Over, nil)
		scaled = dst
	}

	var buf bytes.Buffer
	if hasAlpha(scaled) {
		if err := png.Encode(&buf, scaled); err != nil {
			return nil, fmt.Errorf("thumb: encode png: %w", err)
		}
		return &Result{Data: buf.Bytes(), Width: w, Height: h, Format: "png"}, nil
	}
	if err := jpeg.Encode(&buf, scaled, &jpeg.Options{Quality: 82}); err != nil {
		return nil, fmt.Errorf("thumb: encode jpeg: %w", err)
	}
	return &Result{Data: buf.Bytes(), Width: w, Height: h, Format: "jpeg"}, nil
}

// hasAlpha reports whether the image has any non-opaque pixel. Opaque images
// are stored as JPEG, which is several times smaller and faster to encode.
func hasAlpha(img image.Image) bool {
	switch img.(type) {
	case *image.YCbCr, *image.Gray, *image.CMYK:
		return false
	}
	b := img.Bounds()
	// Sampling every pixel of a large image is wasteful; a coarse grid is
	// enough to decide the container format.
	stepX := max(1, b.Dx()/64)
	stepY := max(1, b.Dy()/64)
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			_, _, _, a := img.At(x, y).RGBA()
			if a < 0xffff {
				return true
			}
		}
	}
	return false
}

// ParseSize is a small helper for settings round-trips.
func ParseSize(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return SnapSize(n)
}
