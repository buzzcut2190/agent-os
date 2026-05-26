package kernel

import (
	"container/list"
	"sync"
	"time"
)

type cacheEntry struct {
	key      string
	value    []byte
	expiresAt time.Time
}

// ContextCache provides cross-agent context caching with LRU eviction and TTL.
type ContextCache struct {
	mu       sync.RWMutex
	maxSize  int
	ttl      time.Duration
	entries  map[string]*list.Element
	lru      *list.List
	hits     int64
	misses   int64
}

// NewContextCache creates a cache with max entries and TTL.
func NewContextCache(maxSize int, ttl time.Duration) *ContextCache {
	if ttl == 0 {
		ttl = 5 * time.Minute
	}
	return &ContextCache{
		maxSize: maxSize,
		ttl:     ttl,
		entries: make(map[string]*list.Element),
		lru:     list.New(),
	}
}

// Get retrieves a cached value. Returns nil, false on miss or expiry.
func (c *ContextCache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.entries[key]
	if !ok {
		c.misses++
		return nil, false
	}
	entry := elem.Value.(*cacheEntry)
	if time.Now().After(entry.expiresAt) {
		c.lru.Remove(elem)
		delete(c.entries, key)
		c.misses++
		return nil, false
	}
	c.lru.MoveToFront(elem)
	c.hits++
	return entry.value, true
}

// Set stores a value in the cache.
func (c *ContextCache) Set(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[key]; ok {
		c.lru.MoveToFront(elem)
		elem.Value.(*cacheEntry).value = value
		elem.Value.(*cacheEntry).expiresAt = time.Now().Add(c.ttl)
		return
	}
	if c.lru.Len() >= c.maxSize {
		oldest := c.lru.Back()
		if oldest != nil {
			c.lru.Remove(oldest)
			delete(c.entries, oldest.Value.(*cacheEntry).key)
		}
	}
	entry := &cacheEntry{key: key, value: value, expiresAt: time.Now().Add(c.ttl)}
	elem := c.lru.PushFront(entry)
	c.entries[key] = elem
}

// Stats returns hit/miss counts.
func (c *ContextCache) Stats() (hits, misses int64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits, c.misses
}

// Clear removes all entries.
func (c *ContextCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*list.Element)
	c.lru.Init()
}
