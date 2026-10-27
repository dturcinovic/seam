package note

import (
	"errors"
	"fmt"
	"time"

	"github.com/dturcinovic/seam/internal/domain/change"
)

// Limits of the book; anything beyond is refused with a named error.
const (
	// MaxText is the most bytes of a note's text.
	MaxText = 8 << 10
	// MaxLines is the most lines a selection holds.
	MaxLines = 200
	// MaxNotes is the most notes not resolved a book holds. Resolved ones
	// do not count, as nothing removes them; MaxFile bounds them all.
	MaxNotes = 2000
	// MaxFile is the most bytes the stored book may take.
	MaxFile = 16 << 20
	// contextLines is how many lines before and after a selection are kept
	// to find it again.
	contextLines = 3
)

// Errors of the book and of its storage.
var (
	ErrTextTooLong  = errors.New("note text too long")
	ErrEmptyText    = errors.New("note text empty")
	ErrTooManyLines = errors.New("selection too long")
	ErrTooManyNotes = errors.New("too many notes")
	ErrBookTooLarge = errors.New("notes file too large")
	ErrNoNote       = errors.New("no such note")
	ErrDuplicateID  = errors.New("note id taken")
	ErrInvalidRange = errors.New("invalid selection")
	ErrInvalidKind  = errors.New("invalid note kind")
	ErrNoOrigin     = errors.New("note anchored in no commit, index or worktree")
	// ErrResolved is an answer to a note already resolved.
	ErrResolved = errors.New("note resolved")
	// ErrNotesDamaged is a stored book that cannot be read; it is never
	// written over, as that would lose the notes in it.
	ErrNotesDamaged = errors.New("notes file damaged")
	// ErrNotesVersion is a stored book of a version this seam does not
	// know; it is never written over either.
	ErrNotesVersion = errors.New("notes file of unknown version")
	// ErrNoBook is notes asked for outside a repository.
	ErrNoBook = errors.New("no notes outside a repository")
	// ErrNotesFailed answers every change once the notes' background
	// work has panicked and runs no more.
	ErrNotesFailed = errors.New("notes failed")
)

// ID names a note, a reply, a claim, a batch or a session.
type ID string

// Origin is where the text a note is set on comes from: a commit, the
// index or the worktree. A note shows in every comparison with its origin
// on a side.
type Origin struct {
	Kind   change.SourceKind
	Commit change.ObjectID
}

// OriginOf is the origin of side s; ok is false for a file on disk, which
// is no place for notes.
func OriginOf(s change.Source) (Origin, bool) {
	switch s.Kind() {
	case change.KindCommit:
		return Origin{Kind: change.KindCommit, Commit: s.ID()}, !s.ID().IsZero()
	case change.KindIndex, change.KindWorktree:
		return Origin{Kind: s.Kind()}, true
	case change.KindPath:
	}
	return Origin{}, false
}

func (o Origin) valid() bool {
	switch o.Kind {
	case change.KindCommit:
		return !o.Commit.IsZero()
	case change.KindIndex, change.KindWorktree:
		return true
	case change.KindPath:
	}
	return false
}

// String names the origin as the user would.
func (o Origin) String() string {
	switch o.Kind {
	case change.KindCommit:
		return o.Commit.String()
	case change.KindIndex:
		return "index"
	case change.KindWorktree:
		return "worktree"
	case change.KindPath:
	}
	return "unset"
}

// Kind is what a note asks for.
type Kind uint8

// The kinds of note. The zero value is no valid kind.
const (
	_ Kind = iota
	Question
	Finding
)

// String is the kind's name in storage.
func (k Kind) String() string {
	switch k {
	case Question:
		return "question"
	case Finding:
		return "finding"
	}
	return "unset"
}

// State is where a note stands.
type State uint8

// The states. The zero value is no valid state.
const (
	_ State = iota
	Open
	Resolved
	// Taken is a note an agent session took and has not answered yet.
	Taken
	// Answered is a note an agent answered.
	Answered
	// Sent is a note sent to an agent session in a batch, which no session
	// took yet.
	Sent
)

// Waits reports that a note in s waits for an agent to take it: open, or
// sent and not taken.
func (s State) Waits() bool { return s == Open || s == Sent }

// String is the state's name in storage.
func (s State) String() string {
	switch s {
	case Open:
		return "open"
	case Resolved:
		return "resolved"
	case Taken:
		return "taken"
	case Answered:
		return "answered"
	case Sent:
		return "sent"
	}
	return "unset"
}

// Note is a question or finding at an anchor. Comparison describes the
// comparison it was set in, for people and agents only: the note belongs
// to the anchor's origin.
type Note struct {
	ID         ID
	Kind       Kind
	Anchor     Anchor
	Comparison string
	Text       string
	State      State
	Created    time.Time
	// Claim is the taking of a note Taken; Batch belongs to sending it,
	// which the book keeps as it is.
	Claim   *Claim
	Batch   ID
	Replies []Reply
}

// Reply is an answer to a note.
type Reply struct {
	ID     ID
	Author string
	Claim  ID
	Text   string
	At     time.Time
}

// Claim is an agent session having taken a note.
type Claim struct {
	ID      ID
	Session ID
	At      time.Time
}

// Batch is a set of notes sent to one agent session.
type Batch struct {
	ID      ID
	Session ID
	Notes   []ID
	At      time.Time
}

// Session is an agent session that reads notes.
type Session struct {
	ID      ID
	Client  string
	PID     int
	Started time.Time
}

// Draft is a note as the user writes it, before the book takes it.
type Draft struct {
	Kind       Kind
	Anchor     Anchor
	Comparison string
	Text       string
}

// Book holds all notes of a worktree, with the sessions and batches of the
// agents that read them.
type Book struct {
	Notes    []Note
	Sessions []Session
	Batches  []Batch
}

// Add takes d as a new open note named id, set at at.
func (b *Book) Add(d Draft, id ID, at time.Time) error {
	switch {
	case d.Text == "":
		return ErrEmptyText
	case len(d.Text) > MaxText:
		return fmt.Errorf("%w: %d bytes, at most %d", ErrTextTooLong, len(d.Text), MaxText)
	case d.Kind != Question && d.Kind != Finding:
		return fmt.Errorf("%w: %d", ErrInvalidKind, d.Kind)
	case !d.Anchor.Origin.valid():
		return ErrNoOrigin
	case b.unresolved() >= MaxNotes:
		return fmt.Errorf("%w: at most %d not resolved", ErrTooManyNotes, MaxNotes)
	}
	if _, ok := b.find(id); ok {
		return fmt.Errorf("%w: %s", ErrDuplicateID, id)
	}
	b.Notes = append(b.Notes, Note{
		ID: id, Kind: d.Kind, Anchor: d.Anchor, Comparison: d.Comparison, Text: d.Text, State: Open, Created: at,
	})
	return nil
}

// Resolve marks note id done; a note resolved already stays so.
func (b *Book) Resolve(id ID) error {
	i, ok := b.find(id)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNoNote, id)
	}
	b.Notes[i].State, b.Notes[i].Claim = Resolved, nil
	return nil
}

func (b *Book) unresolved() int {
	n := 0
	for i := range b.Notes {
		if b.Notes[i].State != Resolved {
			n++
		}
	}
	return n
}

// index maps each note's ID to its place in Notes, for a loop over IDs
// that would else scan the book for each.
func (b *Book) index() map[ID]int {
	at := make(map[ID]int, len(b.Notes))
	for i := range b.Notes {
		at[b.Notes[i].ID] = i
	}
	return at
}

func (b *Book) find(id ID) (int, bool) {
	for i := range b.Notes {
		if b.Notes[i].ID == id {
			return i, true
		}
	}
	return 0, false
}
