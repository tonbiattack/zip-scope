package prefetch

import (
	"container/list"
	"sync"
)

// MemoryCache is a byte-budgeted LRU for decoded images. The budget is a hard
// cap: an entry larger than the whole budget is rejected rather than evicting
// everything else.
type MemoryCache struct {
	mu      sync.Mutex
	limit   int64
	used    int64
	items   map[string]*list.Element
	order   *list.List // front = most recently used
	hits    int64
	misses  int64
	evicted int64
}

type memEntry struct {
	key  string
	size int64
	val  any
}

// NewMemoryCache creates a cache with the given byte budget.
func NewMemoryCache(limit int64) *MemoryCache {
	if limit < 0 {
		limit = 0
	}
	return &MemoryCache{
		limit: limit,
		items: make(map[string]*list.Element),
		order: list.New(),
	}
}

// SetLimit changes the budget and evicts until the cache fits.
func (c *MemoryCache) SetLimit(limit int64) {
	if limit < 0 {
		limit = 0
	}
	c.mu.Lock()
	c.limit = limit
	c.evictLocked(0)
	c.mu.Unlock()
}

// Limit returns the current budget.
func (c *MemoryCache) Limit() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.limit
}

// Put stores value under key, charging size bytes against the budget.
func (c *MemoryCache) Put(key string, size int64, value any) bool {
	if size < 0 {
		size = 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if size > c.limit {
		return false
	}
	if el, ok := c.items[key]; ok {
		ent := el.Value.(*memEntry)
		c.used -= ent.size
		ent.size = size
		ent.val = value
		c.used += size
		c.order.MoveToFront(el)
		c.evictLocked(0)
		return true
	}
	el := c.order.PushFront(&memEntry{key: key, size: size, val: value})
	c.items[key] = el
	c.used += size
	c.evictLocked(0)
	return true
}

// Get returns the value for key and marks it most recently used.
func (c *MemoryCache) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		c.misses++
		return nil, false
	}
	c.order.MoveToFront(el)
	c.hits++
	return el.Value.(*memEntry).val, true
}

// Peek returns the value for key without touching its recency.
func (c *MemoryCache) Peek(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	return el.Value.(*memEntry).val, true
}

// Delete removes one entry.
func (c *MemoryCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.removeLocked(el)
	}
}

// Clear drops every entry.
func (c *MemoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*list.Element)
	c.order.Init()
	c.used = 0
}

// evictLocked removes least-recently-used entries until used <= limit. A
// positive reserve keeps that many bytes free for the caller.
func (c *MemoryCache) evictLocked(reserve int64) {
	for c.used+reserve > c.limit && c.order.Len() > 0 {
		el := c.order.Back()
		if el == nil {
			return
		}
		c.removeLocked(el)
		c.evicted++
	}
}

func (c *MemoryCache) removeLocked(el *list.Element) {
	ent := el.Value.(*memEntry)
	c.order.Remove(el)
	delete(c.items, ent.key)
	c.used -= ent.size
}

// Stats reports cache usage.
type CacheStats struct {
	Limit   int64 `json:"limit"`
	Used    int64 `json:"used"`
	Items   int   `json:"items"`
	Hits    int64 `json:"hits"`
	Misses  int64 `json:"misses"`
	Evicted int64 `json:"evicted"`
}

// Stats returns a snapshot of the cache counters.
func (c *MemoryCache) Stats() CacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CacheStats{
		Limit:   c.limit,
		Used:    c.used,
		Items:   c.order.Len(),
		Hits:    c.hits,
		Misses:  c.misses,
		Evicted: c.evicted,
	}
}
