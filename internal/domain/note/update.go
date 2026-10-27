package note

import (
	"github.com/dturcinovic/seam/internal/domain/change"
)

// SnapshotKey names a side of a file as shown: its path, origin and blob.
// The blob names the text, so a placement that comes late is still right
// for every side with the same key.
type SnapshotKey struct {
	Path   string
	Origin Origin
	Blob   change.ObjectID
}

// Op names a change to the book asked for; its outcome comes as Done.
type Op uint64

// UpdateKind tells which fields of an Update are set.
type UpdateKind uint8

// The kinds of update. The zero value is no valid kind.
const (
	_ UpdateKind = iota
	// Placed sets Key and Spots: the notes of that side and where they
	// stand.
	Placed
	// Done sets Op and, if it failed, Err.
	Done
	// StoreStatus sets Heads, all notes in brief, and Err, why the book
	// cannot be read or is not watched, nil once it is again. Live is
	// whether changes from elsewhere show.
	StoreStatus
)

// Spot is a note and its place in a side.
type Spot struct {
	Note  Note
	Place Placement
}

// Head is a note in brief, as the file list and the footer count them.
type Head struct {
	ID     ID
	Path   string
	Origin Origin
	Kind   Kind
	State  State
}

// Update is news from the notes.
type Update struct {
	Kind UpdateKind

	Key   SnapshotKey
	Spots []Spot

	Op  Op
	Err error

	Heads []Head
	Live  bool
}
