package filediff

import (
	"errors"

	"seam/internal/domain/change"
	"seam/internal/domain/diff"
)

// ErrLoaderFailed is the error of every request once the loader has failed:
// its background work panicked and runs no more.
var ErrLoaderFailed = errors.New("file loader failed")

// Gen is a generation of choosing a file. The UI hands out strictly
// increasing ones and shows only the update of the newest, so a late result
// never covers the file chosen since.
type Gen uint64

// Request asks for the diff of File in Comparison, which says where each
// side is read from. Fresh skips the cache even if the file looks unchanged.
type Request struct {
	Gen        Gen
	Comparison change.Comparison
	File       change.ChangedFile
	Fresh      bool
}

// ReadsWorktree reports that the right side is a worktree file, whose stamp
// tells whether it changed; a blob never changes. A side exists if it has a
// path: an untracked file has no mode yet.
func (r Request) ReadsWorktree() bool {
	return r.Comparison.Right.Kind() == change.KindWorktree && r.File.NewPath != ""
}

// Version counts the contents the right side has had in a session. It rises
// exactly when the text changes; zero means the right side has no text to
// show.
type Version uint64

// Snapshot is the loaded diff of a file. Notices say what it leaves out; a
// side that is too large or binary leaves the diff empty.
type Snapshot struct {
	File    change.ChangedFile
	Diff    diff.FileDiff
	Version Version
	Notices []Notice
}

// Update is the outcome of a request: the snapshot, or Err if loading failed.
type Update struct {
	Gen      Gen
	Snapshot Snapshot
	Err      error
}
