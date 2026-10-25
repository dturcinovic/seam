package filediff

import (
	"errors"

	"seam/internal/domain/change"
	"seam/internal/domain/diff"
	"seam/internal/domain/hover"
	"seam/internal/domain/syntax"
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

// Snapshot is the loaded diff of a file. Notices say what it leaves out; a
// side that is too large or binary leaves the diff empty. Columns index the
// long lines of the diff's sides, Highlights give their syntax. LeftDoc and
// RightDoc are the sides for the language server: set only for a side shown
// as text that comes from a commit or the worktree, as hovers match the
// code only there; the index has no tree to serve it from.
type Snapshot struct {
	File              change.ChangedFile
	Diff              diff.FileDiff
	Columns           Columns
	Highlights        Highlights
	LeftDoc, RightDoc hover.Document
	Notices           []Notice
}

// Doc is the document of side w.
func (s Snapshot) Doc(w Which) hover.Document {
	if w == OldSide {
		return s.LeftDoc
	}
	return s.RightDoc
}

// Highlights are the tokens of both sides' lines.
type Highlights struct {
	Old, New syntax.Tokens
}

// ApproxBytes is roughly the memory both sides' tokens hold.
func (h Highlights) ApproxBytes() int64 {
	return h.Old.ApproxBytes() + h.New.ApproxBytes()
}

// Update is the outcome of a request: the snapshot, or Err if loading failed.
// File is the file asked for, set either way, as the stream asks for many
// at once.
type Update struct {
	Gen      Gen
	File     change.ChangedFile
	Snapshot Snapshot
	Err      error
}
