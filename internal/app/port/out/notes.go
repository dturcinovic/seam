package out

import (
	"context"

	"github.com/dturcinovic/seam/internal/domain/note"
)

//counterfeiter:generate . NoteStore

// NoteStore keeps the book of notes, shared with other processes.
type NoteStore interface {
	// Load reads the book as it is; a missing one is empty, one that
	// cannot be read is note.ErrNotesDamaged or note.ErrNotesVersion.
	Load(ctx context.Context) (note.Book, error)
	// Change reads the book, hands it to change and writes what change
	// left, all under a lock other processes wait for. An error from
	// change, or a book that cannot be read, writes nothing.
	Change(ctx context.Context, change func(*note.Book) error) error
}

//counterfeiter:generate . NoteWatcher

// NoteWatcher watches the book's file for changes from any process.
type NoteWatcher interface {
	// WatchNotes watches the file named file until ctx ends and tells l.
	// It returns nil once ctx ends and an error if watching fails.
	WatchNotes(ctx context.Context, file string, l NoteListener) error
}

// NoteListener hears from a NoteWatcher, one call at a time.
type NoteListener interface {
	// Changed reports that the file may have changed, and once when the
	// watcher is set up, as what changed before cannot be told. It must
	// not block.
	Changed()
}

//counterfeiter:generate . ProcessProber

// ProcessProber tells whether a process of this machine still runs, for
// the agent sessions in the book that ended without leaving.
type ProcessProber interface {
	// Alive reports whether process pid runs.
	Alive(pid int) bool
}
