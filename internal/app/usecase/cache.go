package usecase

import (
	"container/list"

	"seam/internal/domain/change"
	"seam/internal/domain/filediff"
)

// The bounds of the file diff cache. Both hold at once: many small files
// stop at the count, a few large ones at the bytes.
const (
	cacheEntries = 16
	cacheBytes   = 128 << 20
)

// cached is a loaded file. stamp is the worktree file's when the right side
// came from the worktree, so a later load can tell whether it changed; sum
// is the checksum of the right side's text, which its version stands for;
// late says the time for its colours ran out.
type cached struct {
	snapshot filediff.Snapshot
	stamp    change.Stamp
	sum      uint64
	size     int64
	late     bool
}

// cache keeps the most recently used file diffs within its bounds. Only the
// worker uses it, so it has no lock.
type cache struct {
	entries *lru[change.ChangedFile, *cached]
}

func newCache(maxEntries int, maxBytes int64) *cache {
	return &cache{entries: newLRU[change.ChangedFile, *cached](maxEntries, maxBytes)}
}

func (c *cache) get(f change.ChangedFile) (*cached, bool) { return c.entries.get(f) }

// put stores e. An entry larger than the whole budget is not kept: it would
// evict everything else and itself.
func (c *cache) put(e *cached) {
	if e.size > c.entries.maxBytes {
		c.entries.remove(e.snapshot.File)
		return
	}
	c.entries.put(e.snapshot.File, e, e.size)
}

func (c *cache) remove(f change.ChangedFile) { c.entries.remove(f) }

// lru keeps values by key within a count and a sum of their sizes, the
// least recently used going first. It has no lock; its owner has one or
// is a single goroutine.
type lru[K comparable, V any] struct {
	maxEntries int
	maxBytes   int64
	bytes      int64
	order      *list.List // of *lruEntry, most recent first
	index      map[K]*list.Element
}

type lruEntry[K comparable, V any] struct {
	key   K
	value V
	size  int64
}

func newLRU[K comparable, V any](maxEntries int, maxBytes int64) *lru[K, V] {
	return &lru[K, V]{maxEntries: maxEntries, maxBytes: maxBytes, order: list.New(), index: make(map[K]*list.Element)}
}

// get returns the value of k and makes it the most recent.
func (c *lru[K, V]) get(k K) (V, bool) {
	e, ok := c.index[k]
	if !ok {
		var zero V
		return zero, false
	}
	c.order.MoveToFront(e)
	return e.Value.(*lruEntry[K, V]).value, true
}

// put stores v as the most recent and returns the keys it pushes out. The
// newest stays even beyond the bounds.
func (c *lru[K, V]) put(k K, v V, size int64) (evicted []K) {
	c.remove(k)
	c.index[k] = c.order.PushFront(&lruEntry[K, V]{key: k, value: v, size: size})
	c.bytes += size
	for c.order.Len() > 1 && (c.order.Len() > c.maxEntries || c.bytes > c.maxBytes) {
		oldest := c.order.Back().Value.(*lruEntry[K, V]).key
		c.remove(oldest)
		evicted = append(evicted, oldest)
	}
	return evicted
}

func (c *lru[K, V]) remove(k K) {
	e, ok := c.index[k]
	if !ok {
		return
	}
	c.bytes -= e.Value.(*lruEntry[K, V]).size
	c.order.Remove(e)
	delete(c.index, k)
}
