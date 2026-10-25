package usecase

import (
	"sync"

	"github.com/dturcinovic/seam/internal/domain/hover"
)

// versions are the last texts of the paths' right sides, by checksum, so a
// reload can tell whether the text changed even after the cache dropped
// it. The workers of FileDiffs and Streams share them.
type versions struct {
	mu     sync.Mutex
	byPath map[string]version
}

type version struct {
	n   hover.DocVersion
	sum uint64
}

func newVersions() *versions {
	return &versions{byPath: make(map[string]version)}
}

// of returns the path's version for the text with checksum sum, raising it
// if the text differs from the last one shown.
func (vs *versions) of(path string, sum uint64) hover.DocVersion {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	v, ok := vs.byPath[path]
	if !ok || v.sum != sum {
		v = version{n: v.n + 1, sum: sum}
		vs.byPath[path] = v
	}
	return v.n
}
