package note

import (
	"errors"
	"fmt"
	"slices"
)

// Limits of what agents take and answer.
const (
	// MaxReply is the most bytes of an answer.
	MaxReply = 64 << 10
	// MaxTake is the most notes taken at once; more are left for the next
	// call.
	MaxTake = 20
)

// Errors of agent sessions taking and answering notes.
var (
	ErrReplyTooLong = errors.New("answer too long")
	ErrNotTaken     = errors.New("note not taken")
	ErrNoSession    = errors.New("no such agent session")
)

// Reason is why a note asked for was not taken; agents read it as is.
type Reason string

// The reasons a note is not taken. Only open notes are: a question that
// remains after an answer is a new note.
const (
	RefusedUnknown  Reason = "unknown"
	RefusedTaken    Reason = "taken"
	RefusedAnswered Reason = "answered"
	RefusedResolved Reason = "resolved"
)

// Refusal is a note asked for and not taken.
type Refusal struct {
	ID     ID
	Reason Reason
}

// Taking is what one Take took: the notes under Claim, the notes asked for
// and refused, and whether open notes are left past MaxTake.
type Taking struct {
	Claim   Claim
	Notes   []Note
	Refused []Refusal
	More    bool
}

// Answer is what came of a reply: the reply's ID, an earlier one's for the
// same reply again, and whether it came late, under a claim no longer the
// note's.
type Answer struct {
	Reply ID
	Late  bool
}

// Join adds the agent session s.
func (b *Book) Join(s Session) error {
	if _, ok := b.Session(s.ID); ok {
		return fmt.Errorf("%w: session %s", ErrDuplicateID, s.ID)
	}
	b.Sessions = append(b.Sessions, s)
	return nil
}

// Leave removes session id and gives back the notes it took, unanswered.
// A session gone already is no error, so leaving twice does no harm.
func (b *Book) Leave(id ID) {
	b.Sessions = slices.DeleteFunc(b.Sessions, func(s Session) bool { return s.ID == id })
	for i := range b.Notes {
		if n := &b.Notes[i]; n.State == Taken && n.takenBy(id) {
			n.giveBack()
		}
	}
}

func (n Note) takenBy(session ID) bool { return n.Claim != nil && n.Claim.Session == session }

func (n *Note) giveBack() { n.State, n.Claim = Open, nil }

// Prune removes the sessions whose process is no longer alive. A session
// that holds a claim stays: its notes show who took them until the user
// gives them back.
func (b *Book) Prune(alive func(pid int) bool) {
	b.Sessions = slices.DeleteFunc(b.Sessions, func(s Session) bool {
		return !alive(s.PID) && !b.claims(s.ID)
	})
}

func (b *Book) claims(session ID) bool {
	return slices.ContainsFunc(b.Notes, func(n Note) bool { return n.takenBy(session) })
}

// Session is the session named id.
func (b *Book) Session(id ID) (Session, bool) {
	return sessionIn(b.Sessions, id)
}

// TakenBy is the session among sessions that holds n's claim; ok is false
// for a note no session holds, or one whose session is gone.
func TakenBy(n Note, sessions []Session) (Session, bool) {
	if n.Claim == nil {
		return Session{}, false
	}
	return sessionIn(sessions, n.Claim.Session)
}

func sessionIn(sessions []Session, id ID) (Session, bool) {
	i := slices.IndexFunc(sessions, func(s Session) bool { return s.ID == id })
	if i < 0 {
		return Session{}, false
	}
	return sessions[i], true
}

// Take takes notes under c, for its session: those of ids, or without ids
// the open ones, at most MaxTake either way, and More says open ones are
// left. Only open notes are taken; the others of ids come back refused.
func (b *Book) Take(ids []ID, c Claim) (Taking, error) {
	if _, ok := b.Session(c.Session); !ok {
		return Taking{}, fmt.Errorf("%w: %s", ErrNoSession, c.Session)
	}
	t := Taking{Claim: c}
	if len(ids) == 0 {
		ids = b.open(MaxTake + 1)
	} else {
		ids = uniq(ids)
	}
	for _, id := range ids {
		i, ok := b.find(id)
		if !ok {
			t.Refused = append(t.Refused, Refusal{ID: id, Reason: RefusedUnknown})
			continue
		}
		n := &b.Notes[i]
		if r, refused := refusal(n.State); refused {
			t.Refused = append(t.Refused, Refusal{ID: id, Reason: r})
			continue
		}
		if len(t.Notes) == MaxTake {
			t.More = true
			continue
		}
		claim := c
		n.State, n.Claim = Taken, &claim
		t.Notes = append(t.Notes, *n)
	}
	return t, nil
}

// open are the first n open notes of the book.
func (b *Book) open(n int) []ID {
	var ids []ID
	for i := 0; i < len(b.Notes) && len(ids) < n; i++ {
		if b.Notes[i].State == Open {
			ids = append(ids, b.Notes[i].ID)
		}
	}
	return ids
}

func refusal(s State) (Reason, bool) {
	switch s {
	case Open:
		return "", false
	case Taken:
		return RefusedTaken, true
	case Answered:
		return RefusedAnswered, true
	case Resolved:
		return RefusedResolved, true
	}
	return RefusedUnknown, true
}

// uniq is ids in their order, each once.
func uniq(ids []ID) []ID {
	seen := make(map[ID]bool, len(ids))
	return slices.DeleteFunc(slices.Clone(ids), func(id ID) bool {
		dup := seen[id]
		seen[id] = true
		return dup
	})
}

// Reply adds r to note id. The same text under the same claim again adds
// nothing and answers with the reply there, so that an agent may repeat a
// reply whose answer it lost. A reply under a claim other than the note's
// comes late: it is kept, but the newer taking stands. Any other answers
// the note.
func (b *Book) Reply(id ID, r Reply) (Answer, error) {
	switch {
	case r.Text == "":
		return Answer{}, ErrEmptyText
	case len(r.Text) > MaxReply:
		return Answer{}, fmt.Errorf("%w: %d bytes, at most %d", ErrReplyTooLong, len(r.Text), MaxReply)
	}
	i, ok := b.find(id)
	if !ok {
		return Answer{}, fmt.Errorf("%w: %s", ErrNoNote, id)
	}
	n := &b.Notes[i]
	if n.State == Resolved {
		return Answer{}, fmt.Errorf("%w: %s", ErrResolved, id)
	}
	late := n.State == Taken && n.Claim != nil && n.Claim.ID != r.Claim
	if j := slices.IndexFunc(n.Replies, func(old Reply) bool { return old.Claim == r.Claim && old.Text == r.Text }); j >= 0 {
		return Answer{Reply: n.Replies[j].ID, Late: late}, nil
	}
	n.Replies = append(n.Replies, r)
	if !late {
		n.State, n.Claim = Answered, nil
	}
	return Answer{Reply: r.ID, Late: late}, nil
}

// Release gives note id back from the session that took it, unanswered.
func (b *Book) Release(id ID) error {
	i, ok := b.find(id)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNoNote, id)
	}
	n := &b.Notes[i]
	if n.State != Taken {
		return fmt.Errorf("%w: %s is %s", ErrNotTaken, id, n.State)
	}
	n.giveBack()
	return nil
}
