package usecase_test

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/app/port/out"
	"github.com/dturcinovic/seam/internal/app/port/out/outfakes"
	"github.com/dturcinovic/seam/internal/app/usecase"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/note"
)

// batchHearer is a listener for session "me" that keeps what it heard.
type batchHearer struct {
	mu     sync.Mutex
	heard  []note.Sending
	unread []error
}

func (h *batchHearer) Session() (note.ID, bool) { return "me", true }

func (h *batchHearer) Sent(_ context.Context, s note.Sending) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.heard = append(h.heard, s)
}

func (h *batchHearer) Unread(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.unread = append(h.unread, err)
}

func (h *batchHearer) batches() []note.ID {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []note.ID
	for _, s := range h.heard {
		out = append(out, s.Batch.ID)
	}
	return out
}

func newBatches(t *testing.T, s *bookStore) (*usecase.Batches, chan out.NoteListener) {
	t.Helper()
	listeners := make(chan out.NoteListener, 1)
	w := &outfakes.FakeNoteWatcher{}
	w.WatchNotesCalls(func(ctx context.Context, _ string, l out.NoteListener) error {
		listeners <- l
		<-ctx.Done()
		return nil
	})
	b := usecase.NewBatches(usecase.NotesSettings{File: "/n.json"}, s, w, nil)
	require.NoError(t, b.Start())
	t.Cleanup(func() { assert.NoError(t, b.Stop(context.Background())) })
	return b, listeners
}

// send sends the waiting notes of s to session as batch id, as seam would.
func (s *bookStore) send(t *testing.T, id, session note.ID) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.book.Send(id, session, time.Time{}, func(int) bool { return true }, change.Head{})
	require.NoError(t, err)
}

func TestABatchIsHandedOnOnceToItsSessionOnly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newBookStore("a", "b")
		for _, id := range []note.ID{"me", "other"} {
			require.NoError(t, s.book.Join(note.Session{ID: id}))
		}
		s.send(t, "old", "other")
		b, listeners := newBatches(t, s)
		h := &batchHearer{}
		b.Follow(h)
		w := <-listeners
		synctest.Wait()
		assert.Empty(t, h.batches(), "a batch of another session")

		s.send(t, "b1", "me")
		w.Changed()
		synctest.Wait()
		w.Changed()
		synctest.Wait()
		require.Equal(t, []note.ID{"b1"}, h.batches(), "a change after the batch tells nothing again")
		assert.Len(t, h.heard[0].Notes, 2)

		s.send(t, "b2", "me")
		w.Changed()
		synctest.Wait()
		assert.Equal(t, []note.ID{"b1", "b2"}, h.batches(), "sending again is a new batch")
	})
}

func TestNothingIsHandedOnWithoutListener(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newBookStore("a")
		require.NoError(t, s.book.Join(note.Session{ID: "me"}))
		b, listeners := newBatches(t, s)
		w := <-listeners
		s.send(t, "b1", "me")
		w.Changed()
		synctest.Wait()

		h := &batchHearer{}
		b.Follow(h)
		synctest.Wait()
		assert.Equal(t, []note.ID{"b1"}, h.batches(), "following tells what waits")

		b.Follow(nil)
		s.send(t, "b2", "me")
		w.Changed()
		synctest.Wait()
		assert.Equal(t, []note.ID{"b1"}, h.batches())
	})
}

func TestABookThatCannotBeReadIsTold(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newBookStore()
		s.LoadReturns(note.Book{}, note.ErrNotesDamaged)
		b, listeners := newBatches(t, s)
		h := &batchHearer{}
		b.Follow(h)
		<-listeners
		synctest.Wait()

		h.mu.Lock()
		defer h.mu.Unlock()
		require.NotEmpty(t, h.unread)
		require.ErrorIs(t, h.unread[0], note.ErrNotesDamaged)
	})
}
