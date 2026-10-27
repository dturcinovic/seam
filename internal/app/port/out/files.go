package out

import (
	"context"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/limits"
)

//counterfeiter:generate . PathLister

// PathLister lists the files that differ between two directories on disk,
// with paths relative to each. A symlink counts as the file it points to:
// git difftool -d puts the worktree side there as links into the worktree.
type PathLister interface {
	ListPaths(ctx context.Context, left, right string) ([]change.ChangedFile, error)
}

//counterfeiter:generate . PathReader

// PathReader reads a file on disk, following symlinks. Like WorktreeReader it
// returns the stamp of what it read, never reads more than max+1 bytes and
// never blocks on a special file.
type PathReader interface {
	ReadPath(ctx context.Context, path string, max limits.FileSize) ([]byte, change.Stamp, error)
	// StatPath returns the stamp of the file without reading it.
	StatPath(ctx context.Context, path string) (change.Stamp, error)
}

//counterfeiter:generate . WorktreeLocator

// WorktreeLocator finds a file on disk in the repository's worktree, where a
// language server can work on it.
type WorktreeLocator interface {
	// LocateInWorktree returns the path, its symlinks followed, relative to
	// the worktree's root; ok is false outside it, in .git, or without one.
	LocateInWorktree(path string) (rel string, ok bool)
}
