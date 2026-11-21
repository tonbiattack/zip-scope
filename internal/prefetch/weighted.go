package prefetch

import (
	"context"
	"sync"
)

// Weighted is a byte-budgeted semaphore. Callers acquire an estimated cost
// before allocating a large buffer and release it afterwards, which keeps the
// total decode working set bounded independently of the worker count.
//
// A request larger than the whole budget is admitted alone: refusing it would
// make the image permanently unviewable, and the pixel limit already caps how
// big a single decode can be.
type Weighted struct {
	mu      sync.Mutex
	cond    *sync.Cond
	limit   int64
	used    int64
	waiting int
}

// NewWeighted creates a semaphore with the given byte budget.
func NewWeighted(limit int64) *Weighted {
	if limit < 1 {
		limit = 1
	}
	w := &Weighted{limit: limit}
	w.cond = sync.NewCond(&w.mu)
	return w
}

// SetLimit changes the budget. Waiters are re-evaluated.
func (w *Weighted) SetLimit(limit int64) {
	if limit < 1 {
		limit = 1
	}
	w.mu.Lock()
	w.limit = limit
	w.cond.Broadcast()
	w.mu.Unlock()
}

// Limit returns the current budget.
func (w *Weighted) Limit() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.limit
}

// Acquire blocks until cost bytes are available or ctx is done.
func (w *Weighted) Acquire(ctx context.Context, cost int64) error {
	if cost < 0 {
		cost = 0
	}
	if cost > w.limit {
		cost = w.limit
	}

	// Cancellation has to wake the condvar, so watch ctx in a goroutine.
	done := make(chan struct{})
	defer close(done)
	if ctx != nil {
		go func() {
			select {
			case <-ctx.Done():
				w.mu.Lock()
				w.cond.Broadcast()
				w.mu.Unlock()
			case <-done:
			}
		}()
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.waiting++
	defer func() { w.waiting-- }()

	for {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		// Admit when the request fits, or when nothing else is running and the
		// request is oversized (it was clamped to the limit above).
		if w.used+cost <= w.limit || w.used == 0 {
			w.used += cost
			return nil
		}
		w.cond.Wait()
	}
}

// Release returns cost bytes to the budget.
func (w *Weighted) Release(cost int64) {
	if cost < 0 {
		cost = 0
	}
	if cost > w.limit {
		cost = w.limit
	}
	w.mu.Lock()
	w.used -= cost
	if w.used < 0 {
		w.used = 0
	}
	w.cond.Broadcast()
	w.mu.Unlock()
}

// Used returns the currently reserved bytes.
func (w *Weighted) Used() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.used
}

// Waiting returns how many callers are blocked.
func (w *Weighted) Waiting() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.waiting
}
