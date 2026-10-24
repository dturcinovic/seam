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
// is the checksum of the right side's text, which its version stands for.
type cached struct {
	snapshot filediff.Snapshot
	stamp    change.Stamp
	sum      uint64
	size     int64
}

// cache keeps the most recently used file diffs within its bounds. Only the
// worker uses it, so it has no lock.
type cache struct {
	maxEntries int
	maxBytes   int64
	bytes      int64
	order      *list.List // of *cached, most recent first
	index      map[change.ChangedFile]*list.Element
}

func newCache(maxEntries int, maxBytes int64) *cache {
	return &cache{maxEntries: maxEntries, maxBytes: maxBytes, order: list.New(), index: make(map[change.ChangedFile]*list.Element)}
}

func (c *cache) get(f change.ChangedFile) (*cached, bool) {
	e, ok := c.index[f]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(e)
	return e.Value.(*cached), true
}

// put stores e and evicts the least recently used until both bounds hold.
// An entry larger than the whole budget is not kept: it would evict
// everything else and itself.
func (c *cache) put(e *cached) {
	c.remove(e.snapshot.File)
	if e.size > c.maxBytes {
		return
	}
	c.index[e.snapshot.File] = c.order.PushFront(e)
	c.bytes += e.size
	for c.order.Len() > c.maxEntries || c.bytes > c.maxBytes {
		c.remove(c.order.Back().Value.(*cached).snapshot.File)
	}
}

func (c *cache) remove(f change.ChangedFile) {
	e, ok := c.index[f]
	if !ok {
		return
	}
	c.bytes -= e.Value.(*cached).size
	c.order.Remove(e)
	delete(c.index, f)
}
