package in

import (
	"context"

	"github.com/dturcinovic/seam/internal/domain/note"
)

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

//counterfeiter:generate . NoteReleaser

// NoteReleaser gives back a note an agent session took and did not
// answer, as after the session died. It does not block; the outcome comes
// as note.Done through NoteUpdates.
type NoteReleaser interface {
	// Release makes note id open again.
	Release(id note.ID) note.Op
}

//counterfeiter:generate . NoteDeleter

// NoteDeleter takes a note out of the book for good, in whatever state. It
// does not block; the outcome comes as note.Done through NoteUpdates.
type NoteDeleter interface {
	// Delete takes note id out of the book.
	Delete(id note.ID) note.Op
}

//counterfeiter:generate . NoteSessions

// NoteSessions signs agent sessions in and out of the book. Unlike the
// ports of the TUI, each call returns once the book is written.
type NoteSessions interface {
	// Join signs in this process as a session of client.
	Join(ctx context.Context, client string) (note.Session, error)
	// Leave signs session id out and gives back the notes it took and did
	// not answer.
	Leave(ctx context.Context, id note.ID) error
}

//counterfeiter:generate . NoteAnswerer

// NoteAnswerer lets an agent session take, read and answer notes; each
// call returns once the book is read or written.
type NoteAnswerer interface {
	// Take takes for session the open notes of ids, or without ids the
	// open ones, in one change of the book, so that no other session
	// takes them too.
	Take(ctx context.Context, session note.ID, ids []note.ID) (note.Taking, error)
	// Reply answers note id for session, under claim, the claim it was
	// taken with.
	Reply(ctx context.Context, session, id, claim note.ID, text string) (note.Answer, error)
	// View reads the notes of ids, or without ids all not resolved,
	// taking none, with the sessions of the book.
	View(ctx context.Context, ids []note.ID) (note.Book, error)
}

//counterfeiter:generate . NoteSender

// NoteSender sends the waiting notes to one agent session in a batch. It
// does not block; the outcome comes as note.Done through NoteUpdates.
type NoteSender interface {
	// Send sends the notes open, or sent and not taken, to session.
	Send(session note.ID) note.Op
}

//counterfeiter:generate . BatchTaker

// BatchTaker lets an agent session take the notes of a batch sent to it;
// the call returns once the book is written.
type BatchTaker interface {
	// TakeBatch takes for session the notes of batch still sent in it, if
	// the batch went to session; the others come back refused.
	TakeBatch(ctx context.Context, session, batch note.ID) (note.Taking, error)
}

//counterfeiter:generate . BatchFeed

// BatchFeed tells an agent session of the batches sent to it.
type BatchFeed interface {
	// Follow hands l each batch sent to its session from now on, once; nil
	// stops handing on.
	Follow(l BatchListener)
}

// BatchListener hears from a BatchFeed, one call at a time.
type BatchListener interface {
	// Session is the session the listener speaks for; ok is false while
	// it has none.
	Session() (id note.ID, ok bool)
	// Sent hands on a batch sent to Session with its notes not taken yet.
	Sent(ctx context.Context, s note.Sending)
	// Unread tells why the book could not be read after a change; its
	// batches wait for the next change.
	Unread(err error)
}
