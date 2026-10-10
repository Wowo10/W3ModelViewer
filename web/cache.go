package web

import (
	"container/list"
	"sync"
)

// cacheEntry is one modelCache entry: the parsed model and the byte size
// it counts against the cache's bound.
type cacheEntry struct {
	ckey [16]byte
	m    *loaded
	size int64
}

// modelCache is an LRU cache of parsed models keyed by content key,
// bounded by the sum of its entries' sizes (Server.CacheBound), always
// keeping the newest entry even over bound. Two concurrent misses on the
// same content key may both parse and put (identical result, last insert
// wins); accepted, no singleflight.
type modelCache struct {
	mu    sync.Mutex
	ll    *list.List // front = most recently used
	items map[[16]byte]*list.Element
	size  int64
}

func newModelCache() *modelCache {
	return &modelCache{ll: list.New(), items: map[[16]byte]*list.Element{}}
}

// get returns the cached model for ckey, moving it to the front.
func (c *modelCache) get(ckey [16]byte) (*loaded, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[ckey]
	if !ok {
		return nil, false
	}
	c.ll.MoveToFront(el)
	return el.Value.(*cacheEntry).m, true
}

// put inserts or replaces ckey's entry, then evicts the least recently
// used entries (but never the one just inserted) until the cache's total
// size is at most bound.
func (c *modelCache) put(ckey [16]byte, m *loaded, size int64, bound int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[ckey]; ok {
		c.size -= el.Value.(*cacheEntry).size
		el.Value = &cacheEntry{ckey: ckey, m: m, size: size}
		c.size += size
		c.ll.MoveToFront(el)
	} else {
		el := c.ll.PushFront(&cacheEntry{ckey: ckey, m: m, size: size})
		c.items[ckey] = el
		c.size += size
	}

	for c.size > bound && c.ll.Len() > 1 {
		back := c.ll.Back()
		entry := back.Value.(*cacheEntry)
		c.ll.Remove(back)
		delete(c.items, entry.ckey)
		c.size -= entry.size
	}
}
