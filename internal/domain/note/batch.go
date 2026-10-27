package note

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// MaxBatches is the most batches a book keeps; the oldest go first, and
// taking by one gone fails with ErrNoBatch.
const MaxBatches = 64

// Errors of sending notes to agent sessions.
var (
	ErrNothingToSend = errors.New("no notes to send")
	ErrNoBatch       = errors.New("no such batch")
)

// The reasons a note of a batch is not taken, beside those of Take.
const (
	// RefusedOtherSession is a batch sent to another session.
	RefusedOtherSession Reason = "other_session"
	// RefusedRebatched is a note sent again since, in a newer batch.
	RefusedRebatched Reason = "rebatched"
)

// Sending is a batch with those of its notes still sent, not taken.
type Sending struct {
	Batch Batch
	Notes []Note
}

// Send sends the waiting notes, open or sent and not taken, to session in
// a new batch named id, set at at. The batch holds these notes alone: a
// note set later waits for the next one. A note sent before moves to the
// new batch, so a notice that got lost is made up for without a note
// being taken twice. Session must be one of the book's Targets: a session
// whose process died would never take the batch.
func (b *Book) Send(id, session ID, at time.Time, alive func(pid int) bool) (Batch, error) {
	if !slices.ContainsFunc(b.Targets(alive), func(s Session) bool { return s.ID == session }) {
		return Batch{}, fmt.Errorf("%w: %s", ErrNoSession, session)
	}
	if _, ok := b.batch(id); ok {
		return Batch{}, fmt.Errorf("%w: batch %s", ErrDuplicateID, id)
	}
	bt := Batch{ID: id, Session: session, At: at}
	for i := range b.Notes {
		if n := &b.Notes[i]; n.State.Waits() {
			n.State, n.Batch = Sent, id
			bt.Notes = append(bt.Notes, n.ID)
		}
	}
	if len(bt.Notes) == 0 {
		return Batch{}, ErrNothingToSend
	}
	places := b.index()
	b.Batches = append(slices.DeleteFunc(b.Batches, func(old Batch) bool { return b.done(old, places) }), bt)
	if over := len(b.Batches) - MaxBatches; over > 0 {
		b.Batches = slices.Delete(b.Batches, 0, over)
	}
	return bt, nil
}

// done reports that no note of bt waits or is taken any more; at is the
// book's index. A batch whose notes moved to a newer one stays, so that
// taking by it tells why it takes nothing.
func (b *Book) done(bt Batch, at map[ID]int) bool {
	return !slices.ContainsFunc(bt.Notes, func(id ID) bool {
		i, ok := at[id]
		return ok && (b.Notes[i].State.Waits() || b.Notes[i].State == Taken)
	})
}

// TakeBatch takes under c the notes of batch id still sent in it, at most
// MaxTake, if it was sent to c's session. The others come back refused:
// all of a batch sent to another session, and those sent again since in a
// newer batch.
func (b *Book) TakeBatch(id ID, c Claim) (Taking, error) {
	if _, ok := b.Session(c.Session); !ok {
		return Taking{}, fmt.Errorf("%w: %s", ErrNoSession, c.Session)
	}
	bt, ok := b.batch(id)
	if !ok {
		return Taking{}, fmt.Errorf("%w: %s", ErrNoBatch, id)
	}
	if bt.Session != c.Session {
		t := Taking{Claim: c}
		for _, n := range bt.Notes {
			t.Refused = append(t.Refused, Refusal{ID: n, Reason: RefusedOtherSession})
		}
		return t, nil
	}
	// Notes this session took already, by an earlier call for the rest of
	// the batch, are no refusal to tell it of.
	at := b.index()
	ids := slices.DeleteFunc(slices.Clone(bt.Notes), func(id ID) bool {
		i, ok := at[id]
		return ok && b.Notes[i].State == Taken && b.Notes[i].takenBy(c.Session)
	})
	return b.takeEach(ids, c, func(n *Note) (Reason, bool) {
		return RefusedRebatched, n.Batch != id
	}), nil
}

// batch is the batch named id; ok is false for none, also for an empty id.
func (b *Book) batch(id ID) (Batch, bool) {
	i := slices.IndexFunc(b.Batches, func(bt Batch) bool { return bt.ID == id })
	if id == "" || i < 0 {
		return Batch{}, false
	}
	return b.Batches[i], true
}

// Sendings are the batches sent to session that still have notes sent and
// not taken, oldest first.
func (b *Book) Sendings(session ID) []Sending {
	var out []Sending
	at := b.index()
	for _, bt := range b.Batches {
		if bt.Session != session {
			continue
		}
		s := Sending{Batch: bt}
		for _, id := range bt.Notes {
			if i, ok := at[id]; ok && b.Notes[i].State == Sent && b.Notes[i].Batch == bt.ID {
				s.Notes = append(s.Notes, b.Notes[i])
			}
		}
		if len(s.Notes) > 0 {
			out = append(out, s)
		}
	}
	return out
}
