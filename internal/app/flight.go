package app

import (
	"sync"
)

// flightGroup collapses concurrent calls for the same key into one execution.
// The first caller runs fn; every other caller waits for its result. This is
// what keeps a burst of requests for the same thumbnail from decoding the same
// image several times and racing on the same cache file.
type flightGroup struct {
	mu    sync.Mutex
	calls map[string]*flightCall
}

type flightCall struct {
	wg  sync.WaitGroup
	val any
	err error
}

func newFlightGroup() *flightGroup {
	return &flightGroup{calls: make(map[string]*flightCall)}
}

// Do runs fn for key, or waits for the in-flight call with the same key.
func (g *flightGroup) Do(key string, fn func() (any, error)) (any, error) {
	g.mu.Lock()
	if c, ok := g.calls[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := &flightCall{}
	c.wg.Add(1)
	g.calls[key] = c
	g.mu.Unlock()

	// Run outside the lock so other keys are not blocked.
	c.val, c.err = fn()
	c.wg.Done()

	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()
	return c.val, c.err
}

// Forget drops any in-flight entry for key. It is used when a cache entry is
// invalidated so the next request regenerates instead of joining a stale call.
func (g *flightGroup) Forget(key string) {
	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()
}
