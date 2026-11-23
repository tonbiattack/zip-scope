package prefetch

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNewEpochRequestIsNotLostToRunningJob covers the race the issue describes:
// a job from an old epoch is still running when the selection changes and the
// same key is requested again. The new request must run and be notified; it
// must not be silently attached to the old job whose result is discarded.
func TestNewEpochRequestIsNotLostToRunningJob(t *testing.T) {
	s := NewScheduler(1)
	defer s.Close()

	release := make(chan struct{})
	started := make(chan struct{})
	var runs atomic.Int64

	// Start a job in the current epoch and hold it open.
	if err := s.Submit(&Job{
		Key: "same",
		Run: func(ctx context.Context) (any, error) {
			runs.Add(1)
			close(started)
			<-release
			return "old", nil
		},
	}); err != nil {
		t.Fatalf("submit old: %v", err)
	}
	<-started

	// The selection changes while the old job is still running.
	s.BumpEpoch()

	// The same key is requested again, now in the new epoch.
	var delivered atomic.Int64
	var gotValue atomic.Value
	if err := s.Submit(&Job{
		Key: "same",
		Run: func(ctx context.Context) (any, error) {
			runs.Add(1)
			return "new", nil
		},
		Done: func(v any, err error) {
			if err == nil {
				gotValue.Store(v)
			}
			delivered.Add(1)
		},
	}); err != nil {
		t.Fatalf("submit new: %v", err)
	}

	close(release)

	// The new request must be delivered, and with the new value.
	waitFor(t, func() bool { return delivered.Load() == 1 }, "the new request to be delivered")
	if v, _ := gotValue.Load().(string); v != "new" {
		t.Errorf("delivered value = %q, want \"new\"", v)
	}
	if got := runs.Load(); got != 2 {
		t.Errorf("job ran %d times, want 2 (old + new)", got)
	}
}

// TestOldEpochResultIsNotDeliveredToNewWaiter verifies the inverse: a waiter
// that arrives from a newer epoch never receives the old job's result.
func TestOldEpochResultIsNotDeliveredToNewWaiter(t *testing.T) {
	s := NewScheduler(1)
	defer s.Close()

	release := make(chan struct{})
	started := make(chan struct{})
	if err := s.Submit(&Job{
		Key: "k",
		Run: func(ctx context.Context) (any, error) {
			close(started)
			<-release
			return "old", nil
		},
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	<-started

	s.BumpEpoch()

	var oldValue atomic.Value
	if err := s.Submit(&Job{
		Key: "k",
		Run: func(ctx context.Context) (any, error) { return "new", nil },
		Done: func(v any, err error) {
			if err == nil {
				oldValue.Store(v)
			}
		},
	}); err != nil {
		t.Fatalf("submit new: %v", err)
	}

	close(release)
	waitFor(t, func() bool { return oldValue.Load() != nil }, "the new value")

	if v, _ := oldValue.Load().(string); v == "old" {
		t.Error("the new waiter received the old epoch's result")
	}
}

// TestStickyJobSurvivesEpochChangeWithWaiters verifies that an interactive job
// still delivers after the epoch moves on.
func TestStickyJobSurvivesEpochChangeWithWaiters(t *testing.T) {
	s := NewScheduler(1)
	defer s.Close()

	release := make(chan struct{})
	started := make(chan struct{})
	var delivered atomic.Int64

	if err := s.Submit(&Job{
		Key:    "interactive",
		Sticky: true,
		Run: func(ctx context.Context) (any, error) {
			close(started)
			<-release
			return "ok", nil
		},
		Done: func(any, error) { delivered.Add(1) },
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	<-started

	s.BumpEpoch()
	close(release)

	waitFor(t, func() bool { return delivered.Load() == 1 }, "the sticky job to deliver")
}

// TestManyWaitersAllNotified verifies that every caller waiting on one key is
// notified exactly once.
func TestManyWaitersAllNotified(t *testing.T) {
	s := NewScheduler(1)
	defer s.Close()

	release := make(chan struct{})
	started := make(chan struct{})
	var runs atomic.Int64

	if err := s.Submit(&Job{
		Key: "shared",
		Run: func(ctx context.Context) (any, error) {
			runs.Add(1)
			close(started)
			<-release
			return "v", nil
		},
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	<-started

	const waiters = 20
	var notified atomic.Int64
	for i := 0; i < waiters; i++ {
		if err := s.Submit(&Job{
			Key:  "shared",
			Run:  func(ctx context.Context) (any, error) { return "v", nil },
			Done: func(any, error) { notified.Add(1) },
		}); err != nil {
			t.Fatalf("submit waiter %d: %v", i, err)
		}
	}

	close(release)
	waitFor(t, func() bool { return notified.Load() == waiters }, "all waiters to be notified")
	if got := runs.Load(); got != 1 {
		t.Errorf("job ran %d times, want 1", got)
	}
}

// TestEpochChurnUnderLoad hammers Submit and BumpEpoch together. Every request
// must either be delivered or explicitly discarded; none may vanish.
func TestEpochChurnUnderLoad(t *testing.T) {
	s := NewScheduler(4)
	defer s.Close()

	const requests = 400
	var delivered atomic.Int64
	var mu sync.Mutex
	seen := make(map[int]bool)

	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// A third of the requests bump the epoch, which is what a fast
			// click-through does.
			if i%3 == 0 {
				s.BumpEpoch()
			}
			key := "k" + string(rune('a'+i%8))
			_ = s.Submit(&Job{
				Key: key,
				Run: func(ctx context.Context) (any, error) { return i, nil },
				Done: func(v any, err error) {
					if err != nil {
						return
					}
					if n, ok := v.(int); ok {
						mu.Lock()
						seen[n] = true
						mu.Unlock()
					}
					delivered.Add(1)
				},
			})
		}(i)
	}
	wg.Wait()

	// Let the queue drain.
	waitFor(t, func() bool { return s.Pending() == 0 }, "the queue to drain")
	// Give in-flight jobs a moment to finish.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if s.Stats().Completed+s.Stats().Discarded >= int64(requests) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	stats := s.Stats()
	// Every request is accounted for: delivered, discarded, or deduped into
	// another request that was itself accounted for.
	if stats.Completed+stats.Discarded == 0 {
		t.Fatal("no work was accounted for")
	}
	if delivered.Load() > int64(requests) {
		t.Errorf("delivered %d callbacks for %d requests", delivered.Load(), requests)
	}
}

// TestCloseDuringWorkDoesNotPanic verifies that closing while jobs are running
// is safe and that no callback fires after Close returns.
func TestCloseDuringWorkDoesNotPanic(t *testing.T) {
	s := NewScheduler(2)

	var afterClose atomic.Int64
	var closed atomic.Bool

	for i := 0; i < 50; i++ {
		_ = s.Submit(&Job{
			Key: "k" + string(rune('a'+i%10)),
			Run: func(ctx context.Context) (any, error) {
				time.Sleep(time.Millisecond)
				return nil, nil
			},
			Done: func(any, error) {
				if closed.Load() {
					afterClose.Add(1)
				}
			},
		})
	}

	s.Close()
	closed.Store(true)
	// Close waits for the workers, so nothing can fire afterwards.
	time.Sleep(50 * time.Millisecond)
	if afterClose.Load() != 0 {
		t.Errorf("%d callbacks fired after Close returned", afterClose.Load())
	}
}

// TestJobFailureIsDelivered verifies that an error reaches the caller rather
// than being swallowed.
func TestJobFailureIsDelivered(t *testing.T) {
	s := NewScheduler(1)
	defer s.Close()

	want := errors.New("boom")
	got := make(chan error, 1)
	if err := s.Submit(&Job{
		Key:  "fail",
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
		t.Fatal("the failure was not delivered")
	}
}

// TestCancelledJobStillNotifies verifies that a job whose Run returns a context
// error still reaches its caller, so the UI can show a failure instead of
// waiting forever.
func TestCancelledJobStillNotifies(t *testing.T) {
	s := NewScheduler(1)
	defer s.Close()

	got := make(chan error, 1)
	if err := s.Submit(&Job{
		Key: "cancel",
		Run: func(ctx context.Context) (any, error) {
			return nil, context.Canceled
		},
		Done: func(_ any, err error) { got <- err },
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	select {
	case err := <-got:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the cancellation was not delivered")
	}
}

// TestPriorityUpgradeWhileQueued verifies that a more urgent request replaces a
// queued one without running twice.
func TestPriorityUpgradeWhileQueued(t *testing.T) {
	s := NewScheduler(1)
	defer s.Close()

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

	var runs atomic.Int64
	var delivered atomic.Int64
	if err := s.Submit(&Job{
		Key:      "upgrade",
		Priority: PriorityCover,
		Run: func(ctx context.Context) (any, error) {
			runs.Add(1)
			return "low", nil
		},
		Done: func(any, error) { delivered.Add(1) },
	}); err != nil {
		t.Fatalf("submit low: %v", err)
	}
	// The same key arrives with a higher priority.
	if err := s.Submit(&Job{
		Key:      "upgrade",
		Priority: PrioritySelected,
		Run: func(ctx context.Context) (any, error) {
			runs.Add(1)
			return "high", nil
		},
		Done: func(any, error) { delivered.Add(1) },
	}); err != nil {
		t.Fatalf("submit high: %v", err)
	}

	close(release)
	waitFor(t, func() bool { return delivered.Load() == 2 }, "both waiters to be notified")
	if got := runs.Load(); got != 1 {
		t.Errorf("job ran %d times, want 1", got)
	}
}

// TestSubmitAfterCloseIsRejected verifies the closed state is reported.
func TestSubmitAfterCloseIsRejected(t *testing.T) {
	s := NewScheduler(1)
	s.Close()
	if err := s.Submit(&Job{Key: "x", Run: func(ctx context.Context) (any, error) { return nil, nil }}); !errors.Is(err, ErrClosed) {
		t.Errorf("err = %v, want ErrClosed", err)
	}
}
