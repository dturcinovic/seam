package usecase

import (
	"cmp"
	"context"
	"crypto/rand"
	"fmt"
	"slices"
	"time"

	"github.com/dturcinovic/seam/internal/app/port/out"
	"github.com/dturcinovic/seam/internal/domain/note"
)

// AgentSettings is the process an agent session runs in.
type AgentSettings struct {
	PID int
}

// NoteAgents implements in.NoteSessions, in.NoteAnswerer and
// in.BatchTaker for an agent session. It has no worker: each call is one
// change of the book, made under the store's lock, so two sessions never
// take the same note.
type NoteAgents struct {
	store out.NoteStore
	procs out.ProcessProber
	pid   int
}

// NewNoteAgents returns the use case over its ports.
func NewNoteAgents(set AgentSettings, s out.NoteStore, p out.ProcessProber) *NoteAgents {
	return &NoteAgents{store: s, procs: p, pid: set.PID}
}

// Join implements in.NoteSessions. It first drops the sessions whose
// process died without leaving; a client without a name is "agent".
func (a *NoteAgents) Join(ctx context.Context, client string) (note.Session, error) {
	s := note.Session{ID: newID(), Client: cmp.Or(client, "agent"), PID: a.pid, Started: time.Now()}
	err := a.store.Change(ctx, func(b *note.Book) error {
		b.Prune(a.procs.Alive)
		return b.Join(s)
	})
	if err != nil {
		return note.Session{}, fmt.Errorf("join notes: %w", err)
	}
	return s, nil
}

// Leave implements in.NoteSessions.
func (a *NoteAgents) Leave(ctx context.Context, id note.ID) error {
	err := a.store.Change(ctx, func(b *note.Book) error {
		b.Leave(id)
		return nil
	})
	if err != nil {
		return fmt.Errorf("leave notes: %w", err)
	}
	return nil
}

// Take implements in.NoteAnswerer.
func (a *NoteAgents) Take(ctx context.Context, session note.ID, ids []note.ID) (note.Taking, error) {
	c := note.Claim{ID: newID(), Session: session, At: time.Now()}
	var t note.Taking
	err := a.store.Change(ctx, func(b *note.Book) (err error) {
		t, err = b.Take(ids, c)
		return err
	})
	if err != nil {
		return note.Taking{}, fmt.Errorf("take notes: %w", err)
	}
	return t, nil
}

// TakeBatch implements in.BatchTaker.
func (a *NoteAgents) TakeBatch(ctx context.Context, session, batch note.ID) (note.Taking, error) {
	c := note.Claim{ID: newID(), Session: session, At: time.Now()}
	var t note.Taking
	err := a.store.Change(ctx, func(b *note.Book) (err error) {
		t, err = b.TakeBatch(batch, c)
		return err
	})
	if err != nil {
		return note.Taking{}, fmt.Errorf("take batch: %w", err)
	}
	return t, nil
}

// Reply implements in.NoteAnswerer; the reply is the session's client's.
func (a *NoteAgents) Reply(ctx context.Context, session, id, claim note.ID, text string) (note.Answer, error) {
	var ans note.Answer
	err := a.store.Change(ctx, func(b *note.Book) (err error) {
		s, ok := b.Session(session)
		if !ok {
			return fmt.Errorf("%w: %s", note.ErrNoSession, session)
		}
		ans, err = b.Reply(id, note.Reply{ID: newID(), Author: s.Client, Claim: claim, Text: text, At: time.Now()})
		return err
	})
	if err != nil {
		return note.Answer{}, fmt.Errorf("reply to note: %w", err)
	}
	return ans, nil
}

// View implements in.NoteAnswerer; ids not in the book are left out.
func (a *NoteAgents) View(ctx context.Context, ids []note.ID) (note.Book, error) {
	b, err := a.store.Load(ctx)
	if err != nil {
		return note.Book{}, fmt.Errorf("read notes: %w", err)
	}
	asked := make(map[note.ID]bool, len(ids))
	for _, id := range ids {
		asked[id] = true
	}
	b.Notes = slices.DeleteFunc(b.Notes, func(n note.Note) bool {
		if len(ids) > 0 {
			return !asked[n.ID]
		}
		return n.State == note.Resolved
	})
	return note.Book{Notes: b.Notes, Sessions: b.Sessions}, nil
}

func newID() note.ID { return note.ID(rand.Text()) }
