package out

import (
	"context"

	"seam/internal/domain/change"
)

//counterfeiter:generate . ChangeWatcher

// ChangeWatcher watches the files of a repository for changes.
type ChangeWatcher interface {
	// Watch watches git's index and HEAD in p, and with worktree the files
	// under p's root but its ignored directories, until ctx ends, and tells
	// l. It returns nil once ctx ends and an error if watching fails, at
	// the start or later.
	Watch(ctx context.Context, p change.WatchPaths, worktree bool, l WatchListener) error
}

// WatchListener hears from a ChangeWatcher. Its calls come from the
// watcher's goroutine, one at a time.
type WatchListener interface {
	// Changed reports a change, without saying which, and once when the
	// watcher is set up, as what changed before cannot be told. It must not
	// block.
	Changed()
	// Ignored returns those of dirs, directories that came into the
	// worktree after the start, that git ignores; the watcher leaves them
	// out. The watcher waits for the answer.
	Ignored(dirs []string) []string
}
