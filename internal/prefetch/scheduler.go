// Package prefetch schedules background image work with priorities, dedupe,
// cancellation and a byte-budgeted memory cache.
//
// The scheduler owns a priority queue, a fixed worker pool, and an epoch
// counter. Bumping the epoch (which the app does whenever the selection
// changes) discards every queued and in-flight prefetch result from the old
// selection, which is what keeps a fast click-through from showing a stale
// image. Interactive requests are marked sticky and always deliver, because the
// browser is the one that decides whether the response is still wanted.
package prefetch

import (
	"container/heap"
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// Priority orders work. Lower numbers run first.
const (
	// PrioritySelected is the image the user is looking at right now.
	PrioritySelected = 0
	// PriorityVisible is a thumbnail inside the current viewport.
	PriorityVisible = 1
	// PriorityNeighbour is the next/previous image, prefetched ahead of use.
	PriorityNeighbour = 2
	// PriorityCover is a neighbouring archive's representative image.
	PriorityCover = 3
)

// ErrClosed is returned by Submit after Close.
var ErrClosed = errors.New("prefetch: scheduler closed")

// Job is one unit of background work.
type Job struct {
	// Key deduplicates work. Two jobs with the same key never run twice; their
	// callbacks are all invoked with the single result.
	Key string
	// Priority is one of the Priority* constants.
	Priority int
	// Epoch is the selection generation this job belongs to.
	Epoch uint64
	// Sticky jobs deliver their result even if the epoch moved on. Interactive
	// requests set this; speculative prefetches do not.
	Sticky bool
	// Run performs the work. It must honour ctx cancellation.
	Run func(ctx context.Context) (any, error)
	// Done receives the result. It is not called for discarded jobs.
	Done func(value any, err error)
}

type jobHeap []*Job

func (h jobHeap) Len() int { return len(h) }
func (h jobHeap) Less(i, j int) bool {
	if h[i].Priority != h[j].Priority {
		return h[i].Priority < h[j].Priority
	}
	return h[i].Key < h[j].Key
}
func (h jobHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *jobHeap) Push(x any)   { *h = append(*h, x.(*Job)) }
func (h *jobHeap) Pop() any {
	old := *h
	n := len(old)
	j := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return j
}

// Scheduler runs jobs on a fixed worker pool.
type Scheduler struct {
	mu      sync.Mutex
	cond    *sync.Cond
	queue   jobHeap
	pending map[string]*Job
	running map[string]bool
	waiters map[string][]func(any, error)
	closed  bool
	epoch   uint64
	workers int
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc

	completed atomic.Int64
	discarded atomic.Int64
	deduped   atomic.Int64
}

// NewScheduler starts workers goroutines. A worker count below one is clamped
// to one.
func NewScheduler(workers int) *Scheduler {
	if workers < 1 {
		workers = 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		pending: make(map[string]*Job),
		running: make(map[string]bool),
		waiters: make(map[string][]func(any, error)),
		workers: workers,
		ctx:     ctx,
		cancel:  cancel,
	}
	s.cond = sync.NewCond(&s.mu)
	for i := 0; i < workers; i++ {
		s.wg.Add(1)
		go s.worker()
	}
	return s
}

// Workers returns the configured worker count.
func (s *Scheduler) Workers() int { return s.workers }

// Epoch returns the current selection generation.
func (s *Scheduler) Epoch() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.epoch
}

// BumpEpoch advances the selection generation and drops every queued
// non-sticky job. Jobs already running finish but their results are discarded
// unless they are sticky.
func (s *Scheduler) BumpEpoch() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.epoch++
	kept := s.queue[:0]
	for _, j := range s.queue {
		if j.Sticky {
			kept = append(kept, j)
			continue
		}
		delete(s.pending, j.Key)
	}
	s.queue = kept
	heap.Init(&s.queue)
	s.cond.Broadcast()
	return s.epoch
}

// Submit enqueues a job. If a job with the same key is already queued or
// running, the new Done callback is attached to it instead of running twice.
func (s *Scheduler) Submit(j *Job) error {
	if j == nil || j.Run == nil {
		return errors.New("prefetch: job has no Run function")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	if j.Epoch == 0 {
		j.Epoch = s.epoch
	}
	if j.Done != nil {
		s.waiters[j.Key] = append(s.waiters[j.Key], j.Done)
	}
	if s.running[j.Key] {
		s.deduped.Add(1)
		return nil
	}
	if existing, ok := s.pending[j.Key]; ok {
		if j.Priority < existing.Priority {
			existing.Priority = j.Priority
			existing.Epoch = j.Epoch
			existing.Run = j.Run
			existing.Sticky = existing.Sticky || j.Sticky
			heap.Init(&s.queue)
		}
		s.deduped.Add(1)
		return nil
	}
	s.pending[j.Key] = j
	heap.Push(&s.queue, j)
	s.cond.Signal()
	return nil
}

// Pending returns how many jobs are queued.
func (s *Scheduler) Pending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.queue)
}

// Stats reports counters useful for tests and diagnostics.
type Stats struct {
	Completed int64 `json:"completed"`
	Discarded int64 `json:"discarded"`
	Deduped   int64 `json:"deduped"`
	Pending   int   `json:"pending"`
	Workers   int   `json:"workers"`
}

// Stats returns a snapshot of the scheduler counters.
func (s *Scheduler) Stats() Stats {
	return Stats{
		Completed: s.completed.Load(),
		Discarded: s.discarded.Load(),
		Deduped:   s.deduped.Load(),
		Pending:   s.Pending(),
		Workers:   s.workers,
	}
}

func (s *Scheduler) worker() {
	defer s.wg.Done()
	for {
		s.mu.Lock()
		for len(s.queue) == 0 && !s.closed {
			s.cond.Wait()
		}
		if s.closed && len(s.queue) == 0 {
			s.mu.Unlock()
			return
		}
		j := heap.Pop(&s.queue).(*Job)
		delete(s.pending, j.Key)
		s.running[j.Key] = true
		epoch := s.epoch
		s.mu.Unlock()

		// A speculative job from an older selection is dropped before it costs
		// any work.
		if !j.Sticky && j.Epoch != epoch {
			s.mu.Lock()
			delete(s.running, j.Key)
			delete(s.waiters, j.Key)
			s.mu.Unlock()
			s.discarded.Add(1)
			continue
		}

		value, err := j.Run(s.ctx)

		s.mu.Lock()
		delete(s.running, j.Key)
		stale := !j.Sticky && j.Epoch != s.epoch
		callbacks := s.waiters[j.Key]
		delete(s.waiters, j.Key)
		s.mu.Unlock()

		if stale {
			s.discarded.Add(1)
			continue
		}
		s.completed.Add(1)
		for _, cb := range callbacks {
			cb(value, err)
		}
	}
}

// Close stops the workers and waits for them. Queued jobs are dropped.
func (s *Scheduler) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.queue = nil
	s.pending = make(map[string]*Job)
	s.waiters = make(map[string][]func(any, error))
	s.cond.Broadcast()
	s.mu.Unlock()

	s.cancel()
	s.wg.Wait()
}
