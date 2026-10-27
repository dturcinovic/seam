package in

import "github.com/dturcinovic/seam/internal/domain/note"

//counterfeiter:generate . NoteWriter

// NoteWriter changes the book of notes. Neither call blocks; each returns
// at once, and its outcome comes as note.Done through NoteUpdates.
type NoteWriter interface {
	// Add takes d as a new open note.
	Add(d note.Draft) note.Op
	// Resolve marks note id done.
	Resolve(id note.ID) note.Op
}

//counterfeiter:generate . NoteReader

// NoteReader finds where notes stand in a side as shown. It does not
// block; the outcome comes as note.Placed through NoteUpdates.
type NoteReader interface {
	// Place asks for the notes of the side k, whose text is lines.
	Place(k note.SnapshotKey, lines []string)
}

//counterfeiter:generate . NoteUpdates

// NoteUpdates hands on what came of the notes: placements, outcomes of
// changes, and the book's state, also when another process changed it.
type NoteUpdates interface {
	// Signal is set, without blocking, after each new update.
	Signal() <-chan struct{}
	// Drain hands over the updates since the last call, oldest first; it
	// computes nothing.
	Drain() []note.Update
}

//counterfeiter:generate . NoteOverview

// NoteOverview shows the whole book, notes of files the comparison does
// not show included. It does not block; the outcome comes as
// note.Overview through NoteUpdates.
type NoteOverview interface {
	// Overview asks for every note of the book.
	Overview()
}
