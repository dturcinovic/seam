package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
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

// noteFakes is a store over a book in memory, which a test may change as
// another process would, and a watcher that hands out its listener.
type noteFakes struct {
	store    *outfakes.FakeNoteStore
	watcher  *outfakes.FakeNoteWatcher
	listener chan out.NoteListener

	mu   sync.Mutex
	book note.Book
	err  error
}

func newNotes(t *testing.T) (*usecase.Notes, *noteFakes) {
	t.Helper()
	f := &noteFakes{store: &outfakes.FakeNoteStore{}, watcher: &outfakes.FakeNoteWatcher{}, listener: make(chan out.NoteListener, 1)}
	f.store.LoadCalls(func(context.Context) (note.Book, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.book, f.err
	})
	f.store.ChangeCalls(func(_ context.Context, change func(*note.Book) error) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.err != nil {
			return f.err
		}
		b := f.book
		b.Notes = append([]note.Note(nil), b.Notes...)
		if err := change(&b); err != nil {
			return err
		}
		f.book = b
		return nil
	})
	f.watcher.WatchNotesCalls(func(ctx context.Context, file string, l out.NoteListener) error {
		assert.Equal(t, "/repo/.git/seam/notes.json", file)
		f.listener <- l
		l.Changed()
		<-ctx.Done()
		return nil
	})
	n := usecase.NewNotes(usecase.NotesSettings{File: "/repo/.git/seam/notes.json"}, f.store, f.watcher, nil)
	require.NoError(t, n.Start())
	t.Cleanup(func() { assert.NoError(t, n.Stop(context.Background())) })
	return n, f
}

// elsewhere changes the book as another process would.
func (f *noteFakes) elsewhere(change func(*note.Book)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	change(&f.book)
}

func lastOf(us []note.Update, k note.UpdateKind) (note.Update, bool) {
	for _, u := range slices.Backward(us) {
		if u.Kind == k {
			return u, true
		}
	}
	return note.Update{}, false
}

func noteDraft(t *testing.T, lines []string, line int) note.Draft {
	t.Helper()
	a, err := note.NewAnchor("a.go", note.Origin{Kind: change.KindWorktree}, change.ObjectID{}, lines, line, line)
	require.NoError(t, err)
	return note.Draft{Kind: note.Question, Anchor: a, Comparison: "HEAD..worktree", Text: "why?"}
}

// worktreeKey is a.go in the worktree, in a version named by v.
func worktreeKey(v byte) note.SnapshotKey {
	blob := change.BlobID(change.SHA1, []byte{v})
	return note.SnapshotKey{Path: "a.go", Origin: note.Origin{Kind: change.KindWorktree}, Blob: blob}
}

func TestANoteAddedIsPlaced(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n, _ := newNotes(t)
		lines := []string{"a", "b", "c"}

		op := n.Add(noteDraft(t, lines, 2))
		synctest.Wait()
		us := n.Drain()
		done, ok := lastOf(us, note.Done)
		require.True(t, ok)
		assert.Equal(t, op, done.Op)
		require.NoError(t, done.Err)
		status, ok := lastOf(us, note.StoreStatus)
		require.True(t, ok)
		require.Len(t, status.Heads, 1)
		assert.Equal(t, note.Head{ID: status.Heads[0].ID, Path: "a.go", Origin: note.Origin{Kind: change.KindWorktree}, Kind: note.Question, State: note.Open}, status.Heads[0])
		assert.True(t, status.Live)

		n.Place(worktreeKey(1), append([]string{"new"}, lines...))
		synctest.Wait()
		placed, ok := lastOf(n.Drain(), note.Placed)
		require.True(t, ok)
		assert.Equal(t, worktreeKey(1), placed.Key)
		require.Len(t, placed.Spots, 1)
		assert.Equal(t, note.Placement{Where: note.Exact, Start: 3, End: 3}, placed.Spots[0].Place)
		assert.Equal(t, "why?", placed.Spots[0].Note.Text)

		n.Resolve(placed.Spots[0].Note.ID)
		n.Place(worktreeKey(2), lines)
		synctest.Wait()
		placed, _ = lastOf(n.Drain(), note.Placed)
		assert.Empty(t, placed.Spots, "a resolved note is not shown")
	})
}

func TestOnlyTheSideOfTheNoteShowsIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n, _ := newNotes(t)
		lines := []string{"a"}
		n.Add(noteDraft(t, lines, 1))
		index := worktreeKey(1)
		index.Origin = note.Origin{Kind: change.KindIndex}
		n.Place(index, lines)
		synctest.Wait()
		placed, _ := lastOf(n.Drain(), note.Placed)
		assert.Empty(t, placed.Spots, "a worktree note is not the index's")
	})
}

func TestANoteFromElsewhereShowsWithoutAsking(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n, f := newNotes(t)
		l := <-f.listener
		synctest.Wait()
		n.Drain()

		lines := []string{"a", "b"}
		f.elsewhere(func(b *note.Book) {
			require.NoError(t, b.Add(noteDraft(t, lines, 1), "x", time.Time{}))
		})
		l.Changed()
		synctest.Wait()
		status, ok := lastOf(n.Drain(), note.StoreStatus)
		require.True(t, ok)
		require.Len(t, status.Heads, 1)
		assert.Equal(t, note.ID("x"), status.Heads[0].ID)
	})
}

func TestADamagedBookIsReportedAndNotChanged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		damaged := fmt.Errorf("read notes: %w", note.ErrNotesDamaged)
		n, f := newNotes(t)
		f.mu.Lock()
		f.err = damaged
		f.mu.Unlock()
		(<-f.listener).Changed()
		synctest.Wait()
		status, ok := lastOf(n.Drain(), note.StoreStatus)
		require.True(t, ok)
		require.ErrorIs(t, status.Err, note.ErrNotesDamaged)

		n.Add(noteDraft(t, []string{"a"}, 1))
		synctest.Wait()
		done, ok := lastOf(n.Drain(), note.Done)
		require.True(t, ok)
		require.ErrorIs(t, done.Err, note.ErrNotesDamaged)
	})
}

func TestAWatcherThatStopsLeavesTheNotesNotLive(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &noteFakes{store: &outfakes.FakeNoteStore{}, watcher: &outfakes.FakeNoteWatcher{}}
		f.watcher.WatchNotesReturns(errors.New("too many watches"))
		n := usecase.NewNotes(usecase.NotesSettings{File: "/n.json"}, f.store, f.watcher, nil)
		require.NoError(t, n.Start())
		defer func() { assert.NoError(t, n.Stop(context.Background())) }()
		synctest.Wait()
		status, ok := lastOf(n.Drain(), note.StoreStatus)
		require.True(t, ok)
		assert.False(t, status.Live)
		require.NoError(t, status.Err)
	})
}

func TestNoNotesOutsideARepository(t *testing.T) {
	n := usecase.NewNotes(usecase.NotesSettings{}, &outfakes.FakeNoteStore{}, &outfakes.FakeNoteWatcher{}, nil)
	require.NoError(t, n.Start())
	op := n.Add(note.Draft{})
	n.Place(worktreeKey(1), nil)
	us := n.Drain()
	require.Len(t, us, 2)
	assert.Equal(t, note.Update{Kind: note.Done, Op: op, Err: note.ErrNoBook}, us[0])
	assert.Equal(t, note.Update{Kind: note.Placed, Key: worktreeKey(1)}, us[1])
	require.NoError(t, n.Stop(t.Context()))
}
