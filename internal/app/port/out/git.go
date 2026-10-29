package out

import (
	"context"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/limits"
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
// ignore, relative to the repository root. An embedded repository (a nested
// `git init`, a linked worktree) is no file and is not listed.
type UntrackedLister interface {
	ListUntracked(ctx context.Context) ([]string, error)
}

//counterfeiter:generate . BlobReader

// BlobReader reads a blob of the repository. It never reads more than max+1
// bytes: a larger blob yields *change.TooLargeError, an object that is no
// blob change.ErrNotRegular, a missing one change.ErrNotExist.
type BlobReader interface {
	ReadBlob(ctx context.Context, id change.ObjectID, max limits.FileSize) ([]byte, error)
}

//counterfeiter:generate . WorktreeReader

// WorktreeReader reads a file of the worktree with the stamp of what it read.
// A symlink yields its target as text, as git stores it. It never reads more
// than max+1 bytes and never blocks on a special file: a larger file yields
// *change.TooLargeError, anything but a regular file or symlink
// change.ErrNotRegular, a missing path change.ErrNotExist.
type WorktreeReader interface {
	ReadWorktree(ctx context.Context, path string, max limits.FileSize) ([]byte, change.Stamp, error)
}

//counterfeiter:generate . SubmoduleReader

// SubmoduleReader reads a submodule of the worktree as git diff sees it: the
// commit it is at, and dirty if its worktree has changes; untracked files do
// not count. A submodule that is not checked out yields change.ErrNotExist.
type SubmoduleReader interface {
	ReadSubmodule(ctx context.Context, path string) (commit change.ObjectID, dirty bool, err error)
}

//counterfeiter:generate . WorktreeStatter

// WorktreeStatter returns the stamp of a worktree file without reading it.
type WorktreeStatter interface {
	StatWorktree(ctx context.Context, path string) (change.Stamp, error)
}

//counterfeiter:generate . WatchPathsReader

// WatchPathsReader names what a watcher of the repository looks at.
type WatchPathsReader interface {
	// WatchPaths returns the root, git's directory and the directories git
	// ignores, as they are now.
	WatchPaths(ctx context.Context) (change.WatchPaths, error)
}

//counterfeiter:generate . IgnoreChecker

// IgnoreChecker asks git what it ignores.
type IgnoreChecker interface {
	// Ignored returns those of paths, absolute and in the worktree, that git
	// ignores.
	Ignored(ctx context.Context, paths []string) ([]string, error)
}

//counterfeiter:generate . ObjectFormatReader

// ObjectFormatReader names the hash the repository names its objects by.
type ObjectFormatReader interface {
	ObjectFormat(ctx context.Context) (change.ObjectFormat, error)
}

//counterfeiter:generate . HeadReader

// HeadReader tells what HEAD of the worktree stands on, for the notes that
// belong to a branch.
type HeadReader interface {
	// ReadHead returns the branch HEAD stands on, the branch being rebased
	// during a rebase, or else the commit of a detached HEAD.
	ReadHead(ctx context.Context) (change.Head, error)
	// ListBranches returns the names of the local branches.
	ListBranches(ctx context.Context) ([]string, error)
}

//counterfeiter:generate . HeadWatcher

// HeadWatcher watches git's HEAD file of the worktree, which a checkout
// replaces, for the notes that belong to a branch.
type HeadWatcher interface {
	// WatchHead watches the file named file until ctx ends and tells l,
	// once also when the watcher is set up. It returns nil once ctx ends
	// and an error if watching fails.
	WatchHead(ctx context.Context, file string, l HeadListener) error
}

// HeadListener hears from a HeadWatcher, one call at a time.
type HeadListener interface {
	// Changed reports that HEAD may have moved. It must not block.
	Changed()
}
