package thumb

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func solid(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func TestSnapSize(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 96},
		{50, 96},
		{96, 96},
		{100, 128},
		{128, 128},
		{200, 256},
		{256, 256},
		{300, 384},
		{5000, 1024},
	}
	for _, c := range cases {
		if got := SnapSize(c.in); got != c.want {
			t.Errorf("SnapSize(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestGenerateDownscales(t *testing.T) {
	img := solid(1000, 500, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	res, err := Generate(img, 256)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if res.Width != 256 {
		t.Errorf("width = %d, want 256", res.Width)
	}
	if res.Height != 128 {
		t.Errorf("height = %d, want 128", res.Height)
	}
	// An opaque image is stored as JPEG, which is much smaller.
	if res.Format != "jpeg" {
		t.Errorf("format = %q, want jpeg", res.Format)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(res.Data))
	if err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.Width != 256 || cfg.Height != 128 || format != "jpeg" {
		t.Errorf("encoded = %dx%d %s", cfg.Width, cfg.Height, format)
	}
}

func TestGenerateDoesNotUpscale(t *testing.T) {
	img := solid(64, 32, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	res, err := Generate(img, 256)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if res.Width != 64 || res.Height != 32 {
		t.Errorf("size = %dx%d, want 64x32 (no upscale)", res.Width, res.Height)
	}
}

func TestGenerateKeepsAlphaAsPNG(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 200, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			img.Set(x, y, color.NRGBA{R: 255, G: 0, B: 0, A: uint8(x % 256)})
		}
	}
	res, err := Generate(img, 128)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if res.Format != "png" {
		t.Errorf("format = %q, want png for an image with alpha", res.Format)
	}
	if _, format, err := image.Decode(bytes.NewReader(res.Data)); err != nil || format != "png" {
		t.Errorf("decode = %q, %v", format, err)
	}
}

func TestGenerateTallImage(t *testing.T) {
	img := solid(100, 400, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	res, err := Generate(img, 128)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if res.Height != 128 {
		t.Errorf("height = %d, want 128", res.Height)
	}
	if res.Width != 32 {
		t.Errorf("width = %d, want 32", res.Width)
	}
}

func TestGenerateRejectsEmpty(t *testing.T) {
	if _, err := Generate(image.NewRGBA(image.Rect(0, 0, 0, 0)), 128); err == nil {
		t.Fatal("expected an error for an empty image")
	}
}

func TestKeyIsStableAndDistinct(t *testing.T) {
	base := Key{Archive: "fp", Ordinal: 3, CRC32: 0xdeadbeef, Size: 100, Width: 256, Version: Version}
	if base.String() != base.String() {
		t.Error("key is not stable")
	}
	variants := []Key{
		{Archive: "fp2", Ordinal: 3, CRC32: 0xdeadbeef, Size: 100, Width: 256, Version: Version},
		{Archive: "fp", Ordinal: 4, CRC32: 0xdeadbeef, Size: 100, Width: 256, Version: Version},
		{Archive: "fp", Ordinal: 3, CRC32: 0xdeadbeee, Size: 100, Width: 256, Version: Version},
		{Archive: "fp", Ordinal: 3, CRC32: 0xdeadbeef, Size: 101, Width: 256, Version: Version},
		{Archive: "fp", Ordinal: 3, CRC32: 0xdeadbeef, Size: 100, Width: 384, Version: Version},
		{Archive: "fp", Ordinal: 3, CRC32: 0xdeadbeef, Size: 100, Width: 256, Version: Version + 1},
	}
	seen := map[string]bool{base.String(): true}
	for _, v := range variants {
		s := v.String()
		if seen[s] {
			t.Errorf("key collision: %s", s)
		}
		seen[s] = true
	}
}

func TestKeyShardIsTwoLevels(t *testing.T) {
	k := Key{Archive: "fp", Ordinal: 1, Width: 128, Version: Version}
	shard := k.Shard()
	if len(shard) != 5 || shard[2] != '/' {
		t.Errorf("shard = %q, want aa/bb shape", shard)
	}
}

func TestKeyStringIsFilesystemSafe(t *testing.T) {
	k := Key{Archive: "fp/with:bad*chars", Ordinal: 1, Width: 128, Version: Version}
	s := k.String()
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			t.Fatalf("key %q contains a non-hex character %q", s, r)
		}
	}
}

func TestParseSize(t *testing.T) {
	if got := ParseSize("200"); got != 256 {
		t.Errorf("ParseSize(200) = %d, want 256", got)
	}
	if got := ParseSize("nonsense"); got != 0 {
		t.Errorf("ParseSize(nonsense) = %d, want 0", got)
	}
}

func TestGenerateJPEGInput(t *testing.T) {
	// Round-trip through the real decoders so the pipeline is exercised the way
	// the app uses it.
	src := solid(300, 200, color.RGBA{R: 90, G: 120, B: 200, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, nil); err != nil {
		t.Fatalf("encode: %v", err)
	}
	img, _, err := image.Decode(&buf)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	res, err := Generate(img, 192)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if res.Width != 192 {
		t.Errorf("width = %d, want 192", res.Width)
	}
}

func TestGeneratePNGInput(t *testing.T) {
	src := solid(120, 120, color.RGBA{R: 5, G: 6, B: 7, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatalf("encode: %v", err)
	}
	img, _, err := image.Decode(&buf)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	res, err := Generate(img, 96)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if res.Width != 96 || res.Height != 96 {
		t.Errorf("size = %dx%d, want 96x96", res.Width, res.Height)
	}
}
