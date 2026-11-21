package prefetch

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", msg)
}

func TestSchedulerRunsJobs(t *testing.T) {
	s := NewScheduler(2)
	defer s.Close()

	var done atomic.Int64
	for i := 0; i < 10; i++ {
		key := "k" + string(rune('a'+i))
		if err := s.Submit(&Job{
			Key: key,
			Run: func(ctx context.Context) (any, error) { return 1, nil },
			Done: func(any, error) {
				done.Add(1)
			},
		}); err != nil {
			t.Fatalf("submit: %v", err)
		}
	}
	waitFor(t, func() bool { return done.Load() == 10 }, "all jobs to finish")
}

func TestSchedulerDedupesByKey(t *testing.T) {
	s := NewScheduler(1)
	defer s.Close()

	var runs atomic.Int64
	var callbacks atomic.Int64
	// Block the worker so the duplicates queue up behind the first job.
	release := make(chan struct{})
	if err := s.Submit(&Job{
		Key: "same",
		Run: func(ctx context.Context) (any, error) {
			runs.Add(1)
			<-release
			return "v", nil
		},
		Done: func(any, error) { callbacks.Add(1) },
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitFor(t, func() bool { return runs.Load() == 1 }, "the first job to start")

	for i := 0; i < 5; i++ {
		if err := s.Submit(&Job{
			Key: "same",
			Run: func(ctx context.Context) (any, error) {
				runs.Add(1)
				return "v", nil
			},
			Done: func(any, error) { callbacks.Add(1) },
		}); err != nil {
			t.Fatalf("submit dup: %v", err)
		}
	}
	close(release)
	waitFor(t, func() bool { return callbacks.Load() == 6 }, "all callbacks to fire")
	if got := runs.Load(); got != 1 {
		t.Errorf("job ran %d times, want 1", got)
	}
	if got := s.Stats().Deduped; got != 5 {
		t.Errorf("deduped = %d, want 5", got)
	}
}

func TestSchedulerPriorityOrder(t *testing.T) {
	s := NewScheduler(1)
	defer s.Close()

	// Occupy the single worker so the rest queue up.
	release := make(chan struct{})
	started := make(chan struct{})
	if err := s.Submit(&Job{
		Key: "blocker",
		Run: func(ctx context.Context) (any, error) {
			close(started)
			<-release
			return nil, nil
		},
	}); err != nil {
		t.Fatalf("submit blocker: %v", err)
	}
	<-started

	var mu sync.Mutex
	var order []string
	add := func(key string, pri int) {
		if err := s.Submit(&Job{
			Key:      key,
			Priority: pri,
			Run: func(ctx context.Context) (any, error) {
				mu.Lock()
				order = append(order, key)
				mu.Unlock()
				return nil, nil
			},
		}); err != nil {
			t.Fatalf("submit %s: %v", key, err)
		}
	}
	add("cover", PriorityCover)
	add("neighbour", PriorityNeighbour)
	add("visible", PriorityVisible)
	add("selected", PrioritySelected)

	close(release)
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(order) == 4
	}, "queued jobs to run")

	mu.Lock()
	defer mu.Unlock()
	want := []string{"selected", "visible", "neighbour", "cover"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestSchedulerDiscardsStaleEpoch(t *testing.T) {
	s := NewScheduler(1)
	defer s.Close()

	var ran atomic.Int64
	var delivered atomic.Int64

	// Block the worker, queue a speculative job, then bump the epoch.
	release := make(chan struct{})
	started := make(chan struct{})
	if err := s.Submit(&Job{
		Key: "blocker",
		Run: func(ctx context.Context) (any, error) {
			close(started)
			<-release
			return nil, nil
		},
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	<-started

	if err := s.Submit(&Job{
		Key: "stale",
		Run: func(ctx context.Context) (any, error) {
			ran.Add(1)
			return nil, nil
		},
		Done: func(any, error) { delivered.Add(1) },
	}); err != nil {
		t.Fatalf("submit stale: %v", err)
	}

	s.BumpEpoch()
	close(release)

	waitFor(t, func() bool { return s.Stats().Discarded > 0 }, "the stale job to be discarded")
	if ran.Load() != 0 {
		t.Error("a stale job ran after the epoch moved on")
	}
	if delivered.Load() != 0 {
		t.Error("a stale job delivered its result")
	}
}

func TestSchedulerStickyJobSurvivesEpochBump(t *testing.T) {
	s := NewScheduler(1)
	defer s.Close()

	var delivered atomic.Int64
	release := make(chan struct{})
	started := make(chan struct{})
	if err := s.Submit(&Job{
		Key: "blocker",
		Run: func(ctx context.Context) (any, error) {
			close(started)
			<-release
			return nil, nil
		},
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	<-started

	if err := s.Submit(&Job{
		Key:    "interactive",
		Sticky: true,
		Run:    func(ctx context.Context) (any, error) { return "ok", nil },
		Done:   func(any, error) { delivered.Add(1) },
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	s.BumpEpoch()
	close(release)

	waitFor(t, func() bool { return delivered.Load() == 1 }, "the sticky job to deliver")
}

func TestSchedulerCloseStopsWorkers(t *testing.T) {
	s := NewScheduler(2)
	var ran atomic.Int64
	for i := 0; i < 50; i++ {
		_ = s.Submit(&Job{
			Key: "k" + string(rune('a'+i%26)) + string(rune('a'+i/26)),
			Run: func(ctx context.Context) (any, error) {
				ran.Add(1)
				return nil, nil
			},
		})
	}
	s.Close()
	// Submitting after Close must fail rather than panic.
	if err := s.Submit(&Job{Key: "late", Run: func(ctx context.Context) (any, error) { return nil, nil }}); !errors.Is(err, ErrClosed) {
		t.Errorf("err = %v, want ErrClosed", err)
	}
	// Close is idempotent.
	s.Close()
}

func TestSchedulerRejectsJobWithoutRun(t *testing.T) {
	s := NewScheduler(1)
	defer s.Close()
	if err := s.Submit(&Job{Key: "x"}); err == nil {
		t.Fatal("expected an error for a job without Run")
	}
	if err := s.Submit(nil); err == nil {
		t.Fatal("expected an error for a nil job")
	}
}

func TestSchedulerPropagatesErrors(t *testing.T) {
	s := NewScheduler(1)
	defer s.Close()
	want := errors.New("boom")
	got := make(chan error, 1)
	if err := s.Submit(&Job{
		Key:  "err",
		Run:  func(ctx context.Context) (any, error) { return nil, want },
		Done: func(_ any, err error) { got <- err },
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	select {
	case err := <-got:
		if !errors.Is(err, want) {
			t.Errorf("err = %v, want %v", err, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out")
	}
}

func TestSchedulerConcurrentSubmit(t *testing.T) {
	s := NewScheduler(4)
	defer s.Close()

	var wg sync.WaitGroup
	var done atomic.Int64
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := "k" + string(rune('a'+i%26))
			_ = s.Submit(&Job{
				Key: key,
				Run: func(ctx context.Context) (any, error) { return nil, nil },
				Done: func(any, error) {
					done.Add(1)
				},
			})
		}(i)
	}
	wg.Wait()
	// Every distinct key must eventually run; duplicates collapse.
	waitFor(t, func() bool { return s.Pending() == 0 }, "the queue to drain")
	if s.Stats().Workers != 4 {
		t.Errorf("workers = %d, want 4", s.Stats().Workers)
	}
}
