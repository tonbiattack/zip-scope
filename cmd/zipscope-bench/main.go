// Command zipscope-bench generates a synthetic corpus and measures the MVP
// performance targets from docs/SPEC.md. It writes a JSON report and prints a
// human-readable summary.
//
// Usage:
//
//	go run ./cmd/zipscope-bench -out bench-report.json
//	go run ./cmd/zipscope-bench -archives 10000 -images 20 -quick
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/tonbiattack/zip-scope/internal/app"
	"github.com/tonbiattack/zip-scope/internal/bench"
)

func main() {
	var (
		out      = flag.String("out", "bench-report.json", "path for the JSON report")
		archives = flag.Int("archives", 1000, "number of synthetic ZIPs")
		images   = flag.Int("images", 100, "images per ZIP")
		width    = flag.Int("width", 1600, "image width")
		height   = flag.Int("height", 1200, "image height")
		quality  = flag.Int("quality", 85, "JPEG quality")
		seed     = flag.Int64("seed", 1, "corpus seed")
		rounds   = flag.Int("rounds", 30, "measurement rounds per target")
		dir      = flag.String("dir", "", "corpus directory (default: a temp dir)")
		keep     = flag.Bool("keep", false, "keep the generated corpus")
		quick    = flag.Bool("quick", false, "use a small corpus for a smoke run")
	)
	flag.Parse()

	spec := bench.Spec{
		Archives:         *archives,
		ImagesPerArchive: *images,
		Width:            *width,
		Height:           *height,
		Quality:          *quality,
		Seed:             *seed,
	}
	if *quick {
		spec = bench.SmallSpec()
	}

	root := *dir
	if root == "" {
		tmp, err := os.MkdirTemp("", "zipscope-bench-*")
		if err != nil {
			fatal("create corpus dir: %v", err)
		}
		root = tmp
		if !*keep {
			defer os.RemoveAll(root)
		}
	}

	fmt.Printf("generating %d archives x %d images (%dx%d q%d) in %s\n",
		spec.Archives, spec.ImagesPerArchive, spec.Width, spec.Height, spec.Quality, root)
	genStart := time.Now()
	paths, err := bench.Generate(root, spec)
	if err != nil {
		fatal("generate corpus: %v", err)
	}
	genDur := time.Since(genStart)
	fmt.Printf("generated %d archives (%s) in %s\n", len(paths), humanBytes(bench.CorpusBytes(paths)), genDur.Round(time.Millisecond))

	report, err := measure(root, spec, paths, *rounds)
	if err != nil {
		fatal("measure: %v", err)
	}
	report.Extra["corpusGeneration"] = genDur.Round(time.Millisecond).String()
	report.Extra["goVersion"] = runtime.Version()
	report.Extra["numCPU"] = fmt.Sprint(runtime.NumCPU())
	report.Extra["os"] = runtime.GOOS + "/" + runtime.GOARCH

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fatal("marshal report: %v", err)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fatal("write report: %v", err)
	}

	printReport(report)
	fmt.Printf("\nreport written to %s\n", *out)
}

func measure(root string, spec bench.Spec, paths []string, rounds int) (bench.Report, error) {
	report := bench.Report{
		Generated:  time.Now().UTC(),
		Spec:       spec,
		CorpusSize: bench.CorpusBytes(paths),
		ArchiveCnt: len(paths),
		Extra:      map[string]string{},
	}

	dataDir, err := os.MkdirTemp("", "zipscope-bench-data-*")
	if err != nil {
		return report, err
	}
	defer os.RemoveAll(dataDir)

	a, err := app.New(dataDir)
	if err != nil {
		return report, err
	}
	defer a.Shutdown(nil)

	// --- cold scan: no cache at all -------------------------------------
	coldSamples := make([]time.Duration, 0, rounds)
	var firstArchiveID int64
	var firstOrdinal int
	for i := 0; i < rounds; i++ {
		start := time.Now()
		sum, err := a.IndexArchive(paths[i%len(paths)])
		if err != nil {
			return report, err
		}
		coldSamples = append(coldSamples, time.Since(start))
		if i == 0 {
			firstArchiveID = sum.ID
			firstOrdinal = sum.CoverOrdinal
		}
	}
	report.Targets = append(report.Targets,
		bench.Check("cold archive index (central directory)", 250*time.Millisecond, coldSamples))

	// --- warm archive index: metadata already cached ---------------------
	warmSamples := make([]time.Duration, 0, rounds)
	for i := 0; i < rounds; i++ {
		start := time.Now()
		if _, err := a.IndexArchive(paths[i%len(paths)]); err != nil {
			return report, err
		}
		warmSamples = append(warmSamples, time.Since(start))
	}
	report.Targets = append(report.Targets,
		bench.Check("warm archive index (cached metadata)", 50*time.Millisecond, warmSamples))

	// --- cold thumbnail: decode + resize + encode ------------------------
	coldThumb := make([]time.Duration, 0, rounds)
	for i := 0; i < rounds; i++ {
		p := paths[i%len(paths)]
		sum, err := a.IndexArchive(p)
		if err != nil {
			return report, err
		}
		// A distinct width per round would defeat the cache; instead clear the
		// cache so every round is genuinely cold.
		if err := a.ClearCache(); err != nil {
			return report, err
		}
		sum, err = a.IndexArchive(p)
		if err != nil {
			return report, err
		}
		start := time.Now()
		if _, err := a.Thumbnail(sum.ID, sum.CoverOrdinal, 192); err != nil {
			return report, err
		}
		coldThumb = append(coldThumb, time.Since(start))
	}
	report.Targets = append(report.Targets,
		bench.Check("cold thumbnail (decode+resize+encode)", time.Second, coldThumb))

	// --- warm thumbnail: served from the disk cache ----------------------
	sum, err := a.IndexArchive(paths[0])
	if err != nil {
		return report, err
	}
	if _, err := a.Thumbnail(sum.ID, sum.CoverOrdinal, 192); err != nil {
		return report, err
	}
	warmThumb := make([]time.Duration, 0, rounds)
	for i := 0; i < rounds; i++ {
		start := time.Now()
		if _, err := a.Thumbnail(sum.ID, sum.CoverOrdinal, 192); err != nil {
			return report, err
		}
		warmThumb = append(warmThumb, time.Since(start))
	}
	report.Targets = append(report.Targets,
		bench.Check("warm thumbnail (cache hit)", 200*time.Millisecond, warmThumb))

	// --- prefetched full image switch ------------------------------------
	fullSamples := make([]time.Duration, 0, rounds)
	for i := 0; i < rounds; i++ {
		start := time.Now()
		if _, err := a.FullImage(sum.ID, sum.CoverOrdinal); err != nil {
			return report, err
		}
		fullSamples = append(fullSamples, time.Since(start))
	}
	report.Targets = append(report.Targets,
		bench.Check("prefetched full image switch", 100*time.Millisecond, fullSamples))

	// --- cold full image: read + validate --------------------------------
	coldFull := make([]time.Duration, 0, rounds)
	for i := 0; i < rounds; i++ {
		if err := a.ClearCache(); err != nil {
			return report, err
		}
		s, err := a.IndexArchive(paths[i%len(paths)])
		if err != nil {
			return report, err
		}
		start := time.Now()
		if _, err := a.FullImage(s.ID, s.CoverOrdinal); err != nil {
			return report, err
		}
		coldFull = append(coldFull, time.Since(start))
	}
	report.Targets = append(report.Targets,
		bench.Check("cold full image (read+validate)", time.Second, coldFull))

	// --- scan throughput -------------------------------------------------
	scanSamples := make([]time.Duration, 0, 3)
	for i := 0; i < 3; i++ {
		start := time.Now()
		if _, err := a.StartScan(root); err != nil {
			return report, err
		}
		// StartScan is asynchronous; wait for the archive count to settle.
		deadline := time.Now().Add(2 * time.Minute)
		for time.Now().Before(deadline) {
			stats, err := a.GetCacheStats()
			if err != nil {
				return report, err
			}
			if stats.ArchiveCount >= len(paths) {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		scanSamples = append(scanSamples, time.Since(start))
	}
	report.Targets = append(report.Targets,
		bench.Check(fmt.Sprintf("full scan of %d archives", len(paths)), 2*time.Minute, scanSamples))

	// --- memory behaviour under many archives ---------------------------
	before := memUsage()
	for i := 0; i < len(paths) && i < 2000; i++ {
		if _, err := a.IndexArchive(paths[i]); err != nil {
			return report, err
		}
	}
	after := memUsage()
	report.Extra["heapBeforeIndexing"] = humanBytes(int64(before))
	report.Extra["heapAfterIndexing"] = humanBytes(int64(after))
	report.Extra["archivesIndexed"] = fmt.Sprint(min(len(paths), 2000))

	// --- notes -----------------------------------------------------------
	report.Notes = append(report.Notes,
		"Corpus images are synthetic gradients, not photographs; real JPEGs of 1-5 MB decode slower.",
		"Cold measurements clear the whole cache before each round, so they include SQLite writes.",
		"Scan timing waits for the asynchronous walk to index every archive.",
	)
	_ = firstArchiveID
	_ = firstOrdinal
	return report, nil
}

func printReport(r bench.Report) {
	fmt.Printf("\n=== ZipScope benchmark ===\n")
	fmt.Printf("corpus: %d archives, %s\n", r.ArchiveCnt, humanBytes(r.CorpusSize))
	fmt.Printf("%-46s %10s %10s %6s\n", "target", "p95", "goal", "met")
	for _, t := range r.Targets {
		mark := "ok"
		if !t.Met {
			mark = "MISS"
		}
		fmt.Printf("%-46s %10s %10s %6s\n", t.Name, t.Actual.Round(time.Millisecond), t.Goal.Round(time.Millisecond), mark)
	}
	if len(r.Extra) > 0 {
		fmt.Printf("\nextra:\n")
		for k, v := range r.Extra {
			fmt.Printf("  %-24s %s\n", k, v)
		}
	}
	missed := 0
	for _, t := range r.Targets {
		if !t.Met {
			missed++
		}
	}
	if missed > 0 {
		fmt.Printf("\n%d target(s) missed; see the notes in the report.\n", missed)
	}
}

func memUsage() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "zipscope-bench: "+format+"\n", args...)
	os.Exit(1)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ = filepath.Join
