package usecase_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/app/port/out/outfakes"
	"github.com/dturcinovic/seam/internal/app/usecase"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/note"
)

// bookStore is a store over a book in memory, changed one at a time, as
// the lock of the real one does across processes.
type bookStore struct {
	*outfakes.FakeNoteStore
	mu   sync.Mutex
	book note.Book
}

func newBookStore(ids ...note.ID) *bookStore {
	s := &bookStore{FakeNoteStore: &outfakes.FakeNoteStore{}}
	for _, id := range ids {
		_ = s.book.Add(note.Draft{Kind: note.Question, Text: "why " + string(id), Anchor: worktreeAnchor()}, id, time.Time{})
	}
	s.LoadCalls(func(context.Context) (note.Book, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.copied(), nil
	})
	s.ChangeCalls(func(_ context.Context, change func(*note.Book) error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		b := s.copied()
		if err := change(&b); err != nil {
			return err
		}
		s.book = b
		return nil
	})
	return s
}

func worktreeAnchor() note.Anchor { return note.Anchor{Origin: note.Origin{Kind: change.KindWorktree}} }

func (s *bookStore) copied() note.Book { return cloneBook(s.book) }

// cloneBook is b with lists of its own, as a store reads it anew: a change
// that fails leaves the book as it was.
func cloneBook(b note.Book) note.Book {
	return note.Book{Notes: slices.Clone(b.Notes), Sessions: slices.Clone(b.Sessions), Batches: slices.Clone(b.Batches)}
}

func newAgents(s *bookStore, alive ...int) *usecase.NoteAgents {
	return newAgentsOn(s, change.Head{Branch: "main"}, alive...)
}

// newAgentsOn is newAgents in a worktree whose HEAD stands on head.
func newAgentsOn(s *bookStore, head change.Head, alive ...int) *usecase.NoteAgents {
	p := &outfakes.FakeProcessProber{}
	p.AliveCalls(func(pid int) bool { return slices.Contains(alive, pid) })
	h := &outfakes.FakeHeadReader{}
	h.ReadHeadReturns(head, nil)
	return usecase.NewNoteAgents(usecase.AgentSettings{PID: 7}, s, p, h)
}

func TestASessionJoinsAndDropsTheDead(t *testing.T) {
	s := newBookStore()
	s.book.Sessions = []note.Session{{ID: "dead", PID: 3}, {ID: "alive", PID: 4}}
	a := newAgents(s, 4)

	got, err := a.Join(t.Context(), "")
	require.NoError(t, err)

	assert.Equal(t, "agent", got.Client, "a client without a name")
	assert.Equal(t, 7, got.PID)
	assert.False(t, got.Started.IsZero())
	assert.Equal(t, []note.Session{{ID: "alive", PID: 4}, got}, s.book.Sessions)

	require.NoError(t, a.Leave(t.Context(), got.ID))
	assert.Equal(t, []note.Session{{ID: "alive", PID: 4}}, s.book.Sessions)
}

func TestTwoSessionsNeverTakeTheSameNote(t *testing.T) {
	s := newBookStore("a", "b", "c")
	a := newAgents(s, 7)
	one, err := a.Join(t.Context(), "claude-code")
	require.NoError(t, err)
	two, err := a.Join(t.Context(), "opencode")
	require.NoError(t, err)

	var wg sync.WaitGroup
	takings := make([]note.Taking, 2)
	for i, sess := range []note.ID{one.ID, two.ID} {
		wg.Go(func() {
			var err error
			takings[i], err = a.Take(context.Background(), sess, nil)
			assert.NoError(t, err)
		})
	}
	wg.Wait()

	assert.Len(t, append(takings[0].Notes, takings[1].Notes...), 3, "each note once")
	assert.NotEqual(t, takings[0].Claim.ID, takings[1].Claim.ID)
}

func TestAnAgentTakesReadsAndAnswers(t *testing.T) {
	s := newBookStore("a")
	a := newAgents(s, 7)
	sess, err := a.Join(t.Context(), "claude-code")
	require.NoError(t, err)

	took, err := a.Take(t.Context(), sess.ID, []note.ID{"a"})
	require.NoError(t, err)
	require.Len(t, took.Notes, 1)
	assert.Equal(t, sess.ID, took.Claim.Session)

	seen, err := a.View(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, seen.Notes, 1)
	assert.Equal(t, note.Taken, seen.Notes[0].State, "seen as taken, not taken again")
	assert.Equal(t, []note.Session{sess}, seen.Sessions)

	ans, err := a.Reply(t.Context(), sess.ID, "a", took.Claim.ID, "because")
	require.NoError(t, err)
	again, err := a.Reply(t.Context(), sess.ID, "a", took.Claim.ID, "because")
	require.NoError(t, err)
	assert.Equal(t, ans, again)

	got := s.book.Notes[0]
	assert.Equal(t, note.Answered, got.State)
	require.Len(t, got.Replies, 1)
	assert.Equal(t, "claude-code", got.Replies[0].Author)
	assert.Equal(t, "because", got.Replies[0].Text)
}

func TestViewLeavesOutWhatIsNotAskedFor(t *testing.T) {
	s := newBookStore("a", "b", "c")
	require.NoError(t, s.book.Resolve("c"))
	a := newAgents(s)

	all, err := a.View(t.Context(), nil)
	require.NoError(t, err)
	some, err := a.View(t.Context(), []note.ID{"b", "c", "x"})
	require.NoError(t, err)

	ids := func(b note.Book) (out []note.ID) {
		for _, n := range b.Notes {
			out = append(out, n.ID)
		}
		return out
	}
	assert.Equal(t, []note.ID{"a", "b"}, ids(all), "not resolved")
	assert.Equal(t, []note.ID{"b", "c"}, ids(some), "asked for, resolved too")
}

func TestARefusedChangeOfAnAgent(t *testing.T) {
	s := newBookStore("a")
	a := newAgents(s)

	_, err := a.Reply(t.Context(), "nobody", "a", "", "x")
	require.ErrorIs(t, err, note.ErrNoSession)
	_, err = a.Take(t.Context(), "nobody", nil)
	require.ErrorIs(t, err, note.ErrNoSession)

	broken := errors.New("disk full")
	s.ChangeReturns(broken)
	s.LoadReturns(note.Book{}, broken)
	_, err = a.Join(t.Context(), "x")
	require.ErrorIs(t, err, broken)
	require.ErrorIs(t, a.Leave(t.Context(), "x"), broken)
	_, err = a.View(t.Context(), nil)
	require.ErrorIs(t, err, broken)
}

func TestASessionTakesTheBatchSentToIt(t *testing.T) {
	s := newBookStore("a", "b")
	a := newAgents(s, 7)
	me, err := a.Join(t.Context(), "claude-code")
	require.NoError(t, err)
	s.send(t, "b1", me.ID)

	got, err := a.TakeBatch(t.Context(), me.ID, "b1")
	require.NoError(t, err)
	assert.Len(t, got.Notes, 2)
	assert.NotEmpty(t, got.Claim.ID)

	_, err = a.TakeBatch(t.Context(), me.ID, "gone")
	require.ErrorIs(t, err, note.ErrNoBatch)
}

func TestAnAgentLeavesTheNotesOfOtherBranchesUnlessNamed(t *testing.T) {
	s := newBookStore()
	for id, branch := range map[note.ID]string{"here": "main", "there": "feature"} {
		d := note.Draft{Kind: note.Question, Text: "why", Anchor: worktreeAnchor(), On: change.Head{Branch: branch}}
		require.NoError(t, s.book.Add(d, id, time.Time{}))
	}
	a := newAgentsOn(s, change.Head{Branch: "main"}, 7)
	_, err := a.Join(t.Context(), "claude-code")
	require.NoError(t, err)
	session := s.book.Sessions[0].ID

	viewed, err := a.View(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, viewed.Notes, 1)
	assert.Equal(t, note.ID("here"), viewed.Notes[0].ID)

	taken, err := a.Take(t.Context(), session, nil)
	require.NoError(t, err)
	require.Len(t, taken.Notes, 1)
	assert.Equal(t, note.ID("here"), taken.Notes[0].ID)

	named, err := a.Take(t.Context(), session, []note.ID{"there"})
	require.NoError(t, err)
	require.Len(t, named.Notes, 1, "a note named by id is taken wherever it belongs")
}

func TestAnAgentIsToldWhenHEADCannotBeRead(t *testing.T) {
	s := newBookStore("a")
	h := &outfakes.FakeHeadReader{}
	h.ReadHeadReturns(change.Head{}, errors.New("exit 128"))
	a := usecase.NewNoteAgents(usecase.AgentSettings{PID: 7}, s, &outfakes.FakeProcessProber{}, h)
	_, err := a.Join(t.Context(), "claude-code")
	require.NoError(t, err)

	_, err = a.Take(t.Context(), s.book.Sessions[0].ID, nil)
	require.ErrorContains(t, err, "read HEAD: exit 128")
	_, err = a.View(t.Context(), nil)
	require.ErrorContains(t, err, "read HEAD: exit 128")

	named, err := a.Take(t.Context(), s.book.Sessions[0].ID, []note.ID{"a"})
	require.NoError(t, err, "named by id, HEAD does not matter")
	assert.Len(t, named.Notes, 1)
}
