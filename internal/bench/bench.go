// Package bench generates synthetic archives and measures the viewer's
// performance targets. It is used by the `zipscope-bench` command and by the Go
// benchmarks, so the numbers in the README can be reproduced.
package bench

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Spec describes a synthetic corpus.
type Spec struct {
	// Archives is the number of ZIP files to create.
	Archives int
	// ImagesPerArchive is the number of JPEGs inside each ZIP.
	ImagesPerArchive int
	// Width and Height are the pixel dimensions of each generated image.
	Width  int
	Height int
	// Quality is the JPEG quality, which drives the file size.
	Quality int
	// Seed makes the corpus reproducible.
	Seed int64
}

// DefaultSpec matches the specification's measurement corpus: 1000 ZIPs with
// 100 JPEGs each, sized 1-5 MB per image is impractical to generate in a test,
// so the default uses smaller images and the size is reported honestly.
func DefaultSpec() Spec {
	return Spec{
		Archives:         1000,
		ImagesPerArchive: 100,
		Width:            1600,
		Height:           1200,
		Quality:          85,
		Seed:             1,
	}
}

// SmallSpec is a fast corpus for smoke tests and CI.
func SmallSpec() Spec {
	return Spec{
		Archives:         20,
		ImagesPerArchive: 10,
		Width:            640,
		Height:           480,
		Quality:          80,
		Seed:             1,
	}
}

// Generate creates the corpus under root and returns the archive paths.
func Generate(root string, spec Spec) ([]string, error) {
	if spec.Archives <= 0 || spec.ImagesPerArchive <= 0 {
		return nil, fmt.Errorf("bench: spec must have positive counts")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}

	// Pre-encode a pool of distinct images so generation is dominated by ZIP
	// writing rather than JPEG encoding.
	poolSize := 16
	pool := make([][]byte, poolSize)
	for i := 0; i < poolSize; i++ {
		data, err := encodeSynthetic(spec.Width, spec.Height, spec.Quality, int64(i)+spec.Seed)
		if err != nil {
			return nil, err
		}
		pool[i] = data
	}

	paths := make([]string, spec.Archives)
	var wg sync.WaitGroup
	errs := make(chan error, spec.Archives)
	sem := make(chan struct{}, 4)
	for i := 0; i < spec.Archives; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Spread archives across subdirectories so the scan exercises
			// recursion the way a real library does.
			sub := filepath.Join(root, fmt.Sprintf("set%03d", i/50))
			if err := os.MkdirAll(sub, 0o755); err != nil {
				errs <- err
				return
			}
			p := filepath.Join(sub, fmt.Sprintf("book%04d.zip", i))
			if err := writeArchive(p, spec, pool, i); err != nil {
				errs <- err
				return
			}
			paths[i] = p
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return paths, nil
}

func writeArchive(path string, spec Spec, pool [][]byte, index int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for j := 0; j < spec.ImagesPerArchive; j++ {
		// Deterministic but varied: neighbouring archives reuse different
		// images so a cache hit is meaningful.
		data := pool[(index*7+j*3)%len(pool)]
		w, err := zw.Create(fmt.Sprintf("page%03d.jpg", j+1))
		if err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	return zw.Close()
}

// encodeSynthetic builds a photo-like JPEG with a deterministic pattern.
func encodeSynthetic(w, h, quality int, seed int64) ([]byte, error) {
	rng := rand.New(rand.NewSource(seed))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// A smooth gradient plus noise compresses like a real photo, which keeps
	// the resulting file sizes realistic.
	baseR := uint8(rng.Intn(200))
	baseG := uint8(rng.Intn(200))
	baseB := uint8(rng.Intn(200))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := uint8(rng.Intn(24))
			img.Set(x, y, color.RGBA{
				R: uint8(int(baseR) + x*120/w + int(n)),
				G: uint8(int(baseG) + y*120/h + int(n)),
				B: uint8(int(baseB) + (x+y)*60/(w+h) + int(n)),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Stats summarises a set of samples.
type Stats struct {
	Count int           `json:"count"`
	P50   time.Duration `json:"p50"`
	P95   time.Duration `json:"p95"`
	Min   time.Duration `json:"min"`
	Max   time.Duration `json:"max"`
	Mean  time.Duration `json:"mean"`
}

// Summarize computes percentiles from raw samples. The input is not modified.
func Summarize(samples []time.Duration) Stats {
	if len(samples) == 0 {
		return Stats{}
	}
	sorted := make([]time.Duration, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var total time.Duration
	for _, s := range sorted {
		total += s
	}
	return Stats{
		Count: len(sorted),
		P50:   percentile(sorted, 0.50),
		P95:   percentile(sorted, 0.95),
		Min:   sorted[0],
		Max:   sorted[len(sorted)-1],
		Mean:  total / time.Duration(len(sorted)),
	}
}

// percentile returns the nearest-rank percentile of a sorted slice.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted))*p+0.999999) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// Target is one acceptance threshold from the specification.
type Target struct {
	Name    string        `json:"name"`
	Goal    time.Duration `json:"goal"`
	Actual  time.Duration `json:"actual"`
	Met     bool          `json:"met"`
	Samples int           `json:"samples"`
}

// Check compares a measurement against a goal.
func Check(name string, goal time.Duration, samples []time.Duration) Target {
	s := Summarize(samples)
	return Target{Name: name, Goal: goal, Actual: s.P95, Met: s.P95 <= goal, Samples: s.Count}
}

// Report is the full benchmark output.
type Report struct {
	Generated  time.Time         `json:"generated"`
	Spec       Spec              `json:"spec"`
	CorpusSize int64             `json:"corpusBytes"`
	ArchiveCnt int               `json:"archiveCount"`
	Targets    []Target          `json:"targets"`
	Notes      []string          `json:"notes"`
	Extra      map[string]string `json:"extra,omitempty"`
}

// CorpusBytes sums the size of the generated archives.
func CorpusBytes(paths []string) int64 {
	var total int64
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil {
			total += fi.Size()
		}
	}
	return total
}
