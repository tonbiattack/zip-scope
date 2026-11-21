package prefetch

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryCacheEvictsLRU(t *testing.T) {
	c := NewMemoryCache(100)
	c.Put("a", 40, "A")
	c.Put("b", 40, "B")
	if _, ok := c.Get("a"); !ok {
		t.Fatal("a missing")
	}
	// Adding c forces eviction; b is now the least recently used.
	c.Put("c", 40, "C")
	if _, ok := c.Get("b"); ok {
		t.Error("b should have been evicted")
	}
	if _, ok := c.Get("a"); !ok {
		t.Error("a should have survived")
	}
	if _, ok := c.Get("c"); !ok {
		t.Error("c should be present")
	}
}

func TestMemoryCacheRejectsOversizedEntry(t *testing.T) {
	c := NewMemoryCache(100)
	if c.Put("big", 200, "X") {
		t.Error("an entry larger than the budget was accepted")
	}
	if c.Stats().Items != 0 {
		t.Error("cache is not empty")
	}
}

func TestMemoryCacheReplacesExisting(t *testing.T) {
	c := NewMemoryCache(100)
	c.Put("a", 40, "A")
	c.Put("a", 60, "A2")
	if got := c.Stats().Used; got != 60 {
		t.Errorf("used = %d, want 60", got)
	}
	v, ok := c.Get("a")
	if !ok || v != "A2" {
		t.Errorf("value = %v, %v", v, ok)
	}
}

func TestMemoryCacheSetLimitShrinks(t *testing.T) {
	c := NewMemoryCache(1000)
	for _, k := range []string{"a", "b", "c"} {
		c.Put(k, 300, k)
	}
	c.SetLimit(400)
	if used := c.Stats().Used; used > 400 {
		t.Errorf("used = %d, want <= 400", used)
	}
}

func TestMemoryCacheDeleteAndClear(t *testing.T) {
	c := NewMemoryCache(1000)
	c.Put("a", 10, 1)
	c.Put("b", 10, 2)
	c.Delete("a")
	if _, ok := c.Get("a"); ok {
		t.Error("a still present after Delete")
	}
	c.Clear()
	if c.Stats().Items != 0 || c.Stats().Used != 0 {
		t.Errorf("stats = %+v after Clear", c.Stats())
	}
}

func TestMemoryCacheConcurrent(t *testing.T) {
	c := NewMemoryCache(1 << 20)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := string(rune('a' + i%26))
			c.Put(key, 100, i)
			c.Get(key)
		}(i)
	}
	wg.Wait()
	if used := c.Stats().Used; used > c.Limit() {
		t.Errorf("used = %d exceeds limit %d", used, c.Limit())
	}
}

func TestWeightedAcquireRelease(t *testing.T) {
	w := NewWeighted(100)
	ctx := context.Background()
	if err := w.Acquire(ctx, 60); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if w.Used() != 60 {
		t.Errorf("used = %d, want 60", w.Used())
	}
	w.Release(60)
	if w.Used() != 0 {
		t.Errorf("used = %d, want 0", w.Used())
	}
}

func TestWeightedBlocksUntilReleased(t *testing.T) {
	w := NewWeighted(100)
	ctx := context.Background()
	if err := w.Acquire(ctx, 80); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	acquired := make(chan struct{})
	go func() {
		if err := w.Acquire(ctx, 50); err == nil {
			close(acquired)
		}
	}()

	select {
	case <-acquired:
		t.Fatal("second acquire did not block")
	case <-time.After(50 * time.Millisecond):
	}

	w.Release(80)
	select {
	case <-acquired:
	case <-time.After(2 * time.Second):
		t.Fatal("second acquire never completed")
	}
}

func TestWeightedHonoursContext(t *testing.T) {
	w := NewWeighted(100)
	if err := w.Acquire(context.Background(), 100); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := w.Acquire(ctx, 50); err == nil {
		t.Fatal("expected a context error")
	}
}

func TestWeightedOversizedRequestIsAdmittedAlone(t *testing.T) {
	w := NewWeighted(100)
	// A request larger than the budget is clamped and admitted when idle, so a
	// single big image is still viewable.
	if err := w.Acquire(context.Background(), 500); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if w.Used() != 100 {
		t.Errorf("used = %d, want the clamped 100", w.Used())
	}
	w.Release(500)
	if w.Used() != 0 {
		t.Errorf("used = %d, want 0", w.Used())
	}
}

func TestWeightedSetLimitWakesWaiters(t *testing.T) {
	w := NewWeighted(100)
	if err := w.Acquire(context.Background(), 100); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	done := make(chan struct{})
	go func() {
		if err := w.Acquire(context.Background(), 50); err == nil {
			close(done)
		}
	}()
	time.Sleep(20 * time.Millisecond)
	w.SetLimit(200)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("waiter was not woken by SetLimit")
	}
}
