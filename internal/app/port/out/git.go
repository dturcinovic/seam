package out

import (
	"context"

	"seam/internal/domain/change"
	"seam/internal/domain/limits"
)

//counterfeiter:generate . RevisionResolver

// RevisionResolver turns a revision as the user names it into a commit.
type RevisionResolver interface {
	// ResolveCommit fails for a revision that names no commit.
	ResolveCommit(ctx context.Context, rev string) (change.ObjectID, error)
}

//counterfeiter:generate . ChangeLister

// ChangeLister lists the files that differ between the sides of a resolved
// comparison, as git sees them; files git does not track are not among them.
type ChangeLister interface {
	ListChanges(ctx context.Context, c change.Comparison) ([]change.ChangedFile, error)
}

//counterfeiter:generate . UntrackedLister

// UntrackedLister lists the worktree files git does not track and does not
// ignore, relative to the repository root.
type UntrackedLister interface {
	ListUntracked(ctx context.Context) ([]string, error)
}

// BlobReader reads a blob of the repository. It never reads more than max+1
// bytes: a larger blob yields *change.TooLargeError, an object that is no
// blob change.ErrNotRegular, a missing one change.ErrNotExist.
type BlobReader interface {
	ReadBlob(ctx context.Context, id change.ObjectID, max limits.FileSize) ([]byte, error)
}

// WorktreeReader reads a file of the worktree with the stamp of what it read.
// A symlink yields its target as text, as git stores it. It never reads more
// than max+1 bytes and never blocks on a special file: a larger file yields
// *change.TooLargeError, anything but a regular file or symlink
// change.ErrNotRegular, a missing path change.ErrNotExist.
type WorktreeReader interface {
	ReadWorktree(ctx context.Context, path string, max limits.FileSize) ([]byte, change.Stamp, error)
}

// WorktreeStatter returns the stamp of a worktree file without reading it.
type WorktreeStatter interface {
	StatWorktree(ctx context.Context, path string) (change.Stamp, error)
}
