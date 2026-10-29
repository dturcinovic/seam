package usecase_test

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/app/usecase"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/note"
)

func TestANoteBelongsToTheBranchItWasSetOn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n, f := newNotes(t)
		lines := []string{"a", "b"}
		n.Add(noteDraft(t, lines, 1))
		synctest.Wait()
		n.Drain()
		f.mu.Lock()
		assert.Equal(t, change.Head{Branch: "main"}, f.book.Notes[0].On)
		f.mu.Unlock()

		f.head.ReadHeadReturns(change.Head{Branch: "feature"}, nil)
		n.Place(worktreeKey(1), lines)
		synctest.Wait()
		us := n.Drain()
		placed, _ := lastOf(us, note.Placed)
		assert.Empty(t, placed.Spots, "lines that read alike on another branch do not show it")
		status, ok := lastOf(us, note.StoreStatus)
		require.True(t, ok, "a checkout hands on the status anew")
		require.Len(t, status.Heads, 1)
		assert.True(t, status.Heads[0].Away)

		f.head.ReadHeadReturns(change.Head{Branch: "main"}, nil)
		n.Place(worktreeKey(1), lines)
		synctest.Wait()
		placed, _ = lastOf(n.Drain(), note.Placed)
		assert.Len(t, placed.Spots, 1, "back on its branch it shows again")
	})
}

func TestSendingLeavesTheNotesOfOtherBranches(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n, f := newNotes(t)
		f.elsewhere(func(b *note.Book) {
			require.NoError(t, b.Join(note.Session{ID: "live", Client: "claude-code", PID: 2}))
			d := noteDraft(t, []string{"a"}, 1)
			d.On = change.Head{Branch: "feature"}
			require.NoError(t, b.Add(d, "there", time.Time{}))
		})
		(<-f.listener).Changed()
		synctest.Wait()
		n.Drain()

		op := n.Send("live")
		synctest.Wait()
		done, ok := lastOf(n.Drain(), note.Done)
		require.True(t, ok)
		assert.Equal(t, op, done.Op)
		require.ErrorIs(t, done.Err, note.ErrNothingToSend)
	})
}

func TestTheOverviewNamesTheBranchesGone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n, f := newNotes(t)
		f.elsewhere(func(b *note.Book) {
			for id, branch := range map[note.ID]string{"here": "main", "gone": "deleted"} {
				d := noteDraft(t, []string{"a"}, 1)
				d.On = change.Head{Branch: branch}
				require.NoError(t, b.Add(d, id, time.Time{}))
			}
		})
		(<-f.listener).Changed()
		n.Overview()
		synctest.Wait()
		o, ok := lastOf(n.Drain(), note.Overview)
		require.True(t, ok)
		assert.Equal(t, []string{"deleted"}, o.Gone)

		f.head.ListBranchesReturns(nil, errors.New("no refs"))
		n.Overview()
		synctest.Wait()
		o, _ = lastOf(n.Drain(), note.Overview)
		assert.Empty(t, o.Gone, "without the branches none is known to be gone")
	})
}

func TestAHEADNotReadHidesNoNote(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n, f := newNotes(t)
		f.elsewhere(func(b *note.Book) {
			d := noteDraft(t, []string{"a"}, 1)
			d.On = change.Head{Branch: "feature"}
			require.NoError(t, b.Add(d, "there", time.Time{}))
		})
		f.head.ReadHeadReturns(change.Head{}, errors.New("no HEAD"))
		(<-f.listener).Changed()
		n.Place(worktreeKey(1), []string{"a"})
		synctest.Wait()
		us := n.Drain()
		placed, _ := lastOf(us, note.Placed)
		assert.Len(t, placed.Spots, 1)
		status, ok := lastOf(us, note.StoreStatus)
		require.True(t, ok)
		require.ErrorContains(t, status.HeadErr, "read HEAD: no HEAD")
		require.NoError(t, status.Err, "the book itself reads")
		assert.False(t, status.Heads[0].Away)
	})
}

func TestACheckoutSeenOnHEADsFileHandsOnTheStatus(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n, f := newNotesWith(t, usecase.NotesSettings{File: "/repo/.git/seam/notes.json", Head: "/repo/.git/HEAD"})
		head, notes := <-f.headListener, <-f.listener
		f.elsewhere(func(b *note.Book) {
			d := noteDraft(t, []string{"a"}, 1)
			d.On = change.Head{Branch: "main"}
			require.NoError(t, b.Add(d, "here", time.Time{}))
		})
		notes.Changed()
		synctest.Wait()
		reads := f.head.ReadHeadCallCount()
		n.Place(worktreeKey(1), []string{"a"})
		synctest.Wait()
		assert.Equal(t, reads, f.head.ReadHeadCallCount(), "HEAD is read when its file changes, not for each job")
		n.Drain()

		f.head.ReadHeadReturns(change.Head{Branch: "topic"}, nil)
		head.Changed()
		synctest.Wait()
		status, ok := lastOf(n.Drain(), note.StoreStatus)
		require.True(t, ok, "without a job the status goes out")
		require.Len(t, status.Heads, 1)
		assert.True(t, status.Heads[0].Away)
	})
}

func TestWhileHEADCannotBeReadNoNoteIsStoredWithoutItsBranch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n, f := newNotes(t)
		f.elsewhere(func(b *note.Book) {
			require.NoError(t, b.Join(note.Session{ID: "live", Client: "claude-code", PID: 2}))
		})
		(<-f.listener).Changed()
		synctest.Wait()
		f.head.ReadHeadReturns(change.Head{}, errors.New("exit 128"))

		for _, do := range []func() note.Op{
			func() note.Op { return n.Add(noteDraft(t, []string{"a"}, 1)) },
			func() note.Op { return n.Send("live") },
		} {
			op := do()
			synctest.Wait()
			done, ok := lastOf(n.Drain(), note.Done)
			require.True(t, ok)
			assert.Equal(t, op, done.Op)
			require.ErrorContains(t, done.Err, "read HEAD: exit 128")
		}

		f.head.ReadHeadReturns(change.Head{Branch: "main"}, nil)
		n.Place(worktreeKey(1), []string{"a"})
		synctest.Wait()
		status, ok := lastOf(n.Drain(), note.StoreStatus)
		require.True(t, ok, "a HEAD not read is read again at the next job")
		require.NoError(t, status.HeadErr)
	})
}

func TestAddReadsHEADThoughItsFileSaidNothing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n, f := newNotesWith(t, usecase.NotesSettings{File: "/repo/.git/seam/notes.json", Head: "/repo/.git/HEAD"})
		<-f.headListener
		<-f.listener
		synctest.Wait()

		// A HEAD kept in a reftable moves without its file changing.
		f.head.ReadHeadReturns(change.Head{Branch: "topic"}, nil)
		n.Add(noteDraft(t, []string{"a"}, 1))
		synctest.Wait()
		f.mu.Lock()
		defer f.mu.Unlock()
		require.Len(t, f.book.Notes, 1)
		assert.Equal(t, change.Head{Branch: "topic"}, f.book.Notes[0].On)
	})
}
