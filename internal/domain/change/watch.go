package change

import (
	"errors"
	"fmt"
)

var (
	// ErrNotWatchable is a comparison whose sides cannot change: two
	// commits.
	ErrNotWatchable = errors.New("nothing to watch")
	// ErrWatcherFailed is the end of the watcher when its background work
	// panicked.
	ErrWatcherFailed = errors.New("watcher failed")
)

// WatchPaths is what a watcher of the repository looks at: the worktree
// under Root, git's own files in GitDir, and the directories git ignores,
// relative to Root with "/" as git lists them.
type WatchPaths struct {
	Root, GitDir string
	Ignored      []string
}

// WatchesWorktree says whether watching c takes in the worktree, or only
// the index and HEAD, which a commit or a checkout changes. Two commits
// never change, so there is nothing to watch.
func WatchesWorktree(c Comparison) (bool, error) {
	switch c.Right.Kind() {
	case KindWorktree:
		return true, nil
	case KindIndex:
		return false, nil
	case KindPath:
		return false, fmt.Errorf("%w: %s and %s are files on disk", ErrNotWatchable, c.Left, c.Right)
	case KindCommit:
	}
	return false, fmt.Errorf("%w: %s and %s are commits", ErrNotWatchable, c.Left, c.Right)
}

// WatchKind says what a WatchUpdate reports.
type WatchKind int

// The kinds of watch update. The zero value is no valid kind.
const (
	_ WatchKind = iota
	// Watching reports that the watcher runs.
	Watching
	// Changed reports that the side being watched changed, once a burst of
	// changes has settled.
	Changed
	// WatchFailed reports that the watcher stopped; Err says why.
	WatchFailed
)

// WatchUpdate is news from the watcher.
type WatchUpdate struct {
	Kind WatchKind
	Err  error
}
