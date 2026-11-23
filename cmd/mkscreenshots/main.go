// Command mkscreenshots generates the sample archives used for the Store
// screenshots.
//
// The images are drawn procedurally, so there is no third-party material and no
// licensing question about what appears in the listing. Each archive is themed
// so the screenshots show a plausible library rather than identical tiles.
package main

import (
	"archive/zip"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
)

// theme is one archive's palette and motif.
type theme struct {
	name   string
	sky    color.RGBA
	ground color.RGBA
	accent color.RGBA
	motif  int
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: mkscreenshots <output-dir>")
		os.Exit(2)
	}
	root := os.Args[1]
	if err := os.MkdirAll(root, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}

	themes := []theme{
		{"01 山の風景", color.RGBA{135, 190, 235, 255}, color.RGBA{74, 124, 90, 255}, color.RGBA{255, 214, 102, 255}, 0},
		{"02 海辺のスケッチ", color.RGBA{120, 200, 220, 255}, color.RGBA{40, 110, 150, 255}, color.RGBA{255, 245, 200, 255}, 1},
		{"03 夜の街", color.RGBA{30, 34, 60, 255}, color.RGBA{18, 20, 34, 255}, color.RGBA{255, 200, 120, 255}, 2},
		{"04 花畑", color.RGBA{245, 220, 235, 255}, color.RGBA{120, 170, 110, 255}, color.RGBA{230, 110, 150, 255}, 3},
		{"05 砂丘", color.RGBA{250, 225, 190, 255}, color.RGBA{210, 170, 120, 255}, color.RGBA{255, 250, 235, 255}, 4},
	}

	for i, t := range themes {
		path := filepath.Join(root, fmt.Sprintf("%s.cbz", t.name))
		if err := writeArchive(path, t, 8+i); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", filepath.Base(path))
	}
}

// writeArchive builds one CBZ with a handful of pages.
func writeArchive(path string, t theme, pages int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for i := 0; i < pages; i++ {
		img := render(t, i, 1200, 1600)
		w, err := zw.Create(fmt.Sprintf("page%02d.jpg", i+1))
		if err != nil {
			return err
		}
		if err := jpeg.Encode(w, img, &jpeg.Options{Quality: 88}); err != nil {
			return err
		}
	}
	return zw.Close()
}

// render draws one page: a gradient sky, a horizon, and a motif that varies per
// theme so the thumbnails are visually distinct.
func render(t theme, page, w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	horizon := h * 55 / 100
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var c color.RGBA
			if y < horizon {
				// Sky: vertical gradient with a slight page-to-page shift.
				f := float64(y) / float64(horizon)
				shift := float64(page) * 0.04
				c = blend(t.sky, lighten(t.sky, 0.35), clamp01(f+shift))
			} else {
				f := float64(y-horizon) / float64(h-horizon)
				c = blend(t.ground, darken(t.ground, 0.25), clamp01(f))
			}
			img.Set(x, y, c)
		}
	}

	switch t.motif {
	case 0: // mountains
		drawTriangle(img, w/2, horizon-40, w*45/100, h*30/100, darken(t.ground, 0.15))
		drawTriangle(img, w/4, horizon-10, w*30/100, h*18/100, darken(t.ground, 0.05))
		drawCircle(img, w*78/100, h*18/100, w/12, t.accent)
	case 1: // sea and sun
		drawCircle(img, w*72/100, h*20/100, w/10, t.accent)
		for i := 0; i < 6; i++ {
			y := horizon + i*h/22
			drawWave(img, y, w, h/60, lighten(t.ground, 0.3))
		}
	case 2: // city at night
		for i := 0; i < 9; i++ {
			bw := w / 14
			bx := i * (w / 9)
			bh := h/4 + (i%3)*h/12
			drawRect(img, bx, horizon-bh, bw, bh, darken(t.ground, 0.1))
			for wy := horizon - bh + 20; wy < horizon-20; wy += 40 {
				for wx := bx + 12; wx < bx+bw-12; wx += 28 {
					if (wx+wy+i)%3 == 0 {
						drawRect(img, wx, wy, 12, 16, t.accent)
					}
				}
			}
		}
	case 3: // flowers
		for i := 0; i < 40; i++ {
			x := (i*137 + page*53) % w
			y := horizon + (i*71)%(h-horizon-40)
			drawCircle(img, x, y, 10+(i%4)*4, t.accent)
		}
	case 4: // dunes
		drawWave(img, horizon+h/12, w, h/18, lighten(t.ground, 0.18))
		drawWave(img, horizon+h/5, w, h/22, lighten(t.ground, 0.3))
		drawCircle(img, w*25/100, h*16/100, w/14, t.accent)
	}

	// A page number, so the grid shows distinct pages.
	drawRect(img, w-140, h-90, 100, 50, color.RGBA{0, 0, 0, 120})
	return img
}

func drawTriangle(img *image.RGBA, cx, baseY, halfW, height int, c color.RGBA) {
	for y := 0; y < height; y++ {
		half := halfW * (height - y) / height
		for x := cx - half; x < cx+half; x++ {
			set(img, x, baseY-height+y, c)
		}
	}
}

func drawCircle(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			if x*x+y*y <= r*r {
				set(img, cx+x, cy+y, c)
			}
		}
	}
}

func drawRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			set(img, xx, yy, c)
		}
	}
}

func drawWave(img *image.RGBA, baseY, w, amp int, c color.RGBA) {
	for x := 0; x < w; x++ {
		y := baseY + int(float64(amp)*math.Sin(float64(x)/60))
		for d := 0; d < 6; d++ {
			set(img, x, y+d, c)
		}
	}
}

func set(img *image.RGBA, x, y int, c color.RGBA) {
	if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
		return
	}
	img.SetRGBA(x, y, c)
}

func blend(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		G: uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		B: uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
		A: 255,
	}
}

func lighten(c color.RGBA, f float64) color.RGBA {
	return blend(c, color.RGBA{255, 255, 255, 255}, f)
}

func darken(c color.RGBA, f float64) color.RGBA {
	return blend(c, color.RGBA{0, 0, 0, 255}, f)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
