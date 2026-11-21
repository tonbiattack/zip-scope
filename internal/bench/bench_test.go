package bench

import (
	"testing"
	"time"
)

func TestSummarize(t *testing.T) {
	samples := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
	}
	s := Summarize(samples)
	if s.Count != 5 {
		t.Errorf("count = %d", s.Count)
	}
	if s.Min != 10*time.Millisecond || s.Max != 50*time.Millisecond {
		t.Errorf("min/max = %v/%v", s.Min, s.Max)
	}
	if s.P50 != 30*time.Millisecond {
		t.Errorf("p50 = %v, want 30ms", s.P50)
	}
	if s.P95 != 50*time.Millisecond {
		t.Errorf("p95 = %v, want 50ms", s.P95)
	}
	if s.Mean != 30*time.Millisecond {
		t.Errorf("mean = %v, want 30ms", s.Mean)
	}
}

func TestSummarizeEmpty(t *testing.T) {
	if s := Summarize(nil); s.Count != 0 {
		t.Errorf("count = %d, want 0", s.Count)
	}
}

func TestSummarizeDoesNotMutateInput(t *testing.T) {
	samples := []time.Duration{30 * time.Millisecond, 10 * time.Millisecond, 20 * time.Millisecond}
	_ = Summarize(samples)
	if samples[0] != 30*time.Millisecond {
		t.Error("input slice was reordered")
	}
}

func TestPercentileNearestRank(t *testing.T) {
	// 100 samples of 1..100 ms: p95 must be the 95th value.
	var samples []time.Duration
	for i := 1; i <= 100; i++ {
		samples = append(samples, time.Duration(i)*time.Millisecond)
	}
	s := Summarize(samples)
	if s.P95 != 95*time.Millisecond {
		t.Errorf("p95 = %v, want 95ms", s.P95)
	}
	if s.P50 != 50*time.Millisecond {
		t.Errorf("p50 = %v, want 50ms", s.P50)
	}
}

func TestCheck(t *testing.T) {
	samples := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}
	if got := Check("fast", 500*time.Millisecond, samples); !got.Met {
		t.Error("target should be met")
	}
	if got := Check("slow", 50*time.Millisecond, samples); got.Met {
		t.Error("target should not be met")
	}
}

func TestGenerateSmallCorpus(t *testing.T) {
	root := t.TempDir()
	spec := SmallSpec()
	spec.Archives = 3
	spec.ImagesPerArchive = 4
	spec.Width = 64
	spec.Height = 48

	paths, err := Generate(root, spec)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(paths) != 3 {
		t.Fatalf("paths = %d, want 3", len(paths))
	}
	if CorpusBytes(paths) == 0 {
		t.Error("corpus is empty")
	}
}

func TestGenerateRejectsBadSpec(t *testing.T) {
	if _, err := Generate(t.TempDir(), Spec{Archives: 0, ImagesPerArchive: 1}); err == nil {
		t.Fatal("expected an error for a zero archive count")
	}
	if _, err := Generate(t.TempDir(), Spec{Archives: 1, ImagesPerArchive: 0}); err == nil {
		t.Fatal("expected an error for a zero image count")
	}
}

func TestGenerateIsReproducible(t *testing.T) {
	spec := Spec{Archives: 2, ImagesPerArchive: 2, Width: 32, Height: 32, Quality: 70, Seed: 42}
	a, err := Generate(t.TempDir(), spec)
	if err != nil {
		t.Fatalf("generate a: %v", err)
	}
	b, err := Generate(t.TempDir(), spec)
	if err != nil {
		t.Fatalf("generate b: %v", err)
	}
	if CorpusBytes(a) != CorpusBytes(b) {
		t.Errorf("corpus sizes differ: %d vs %d", CorpusBytes(a), CorpusBytes(b))
	}
}
