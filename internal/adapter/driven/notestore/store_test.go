package notestore_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/adapter/driven/notestore"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/note"
)

func newStore(t *testing.T) (*notestore.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "git", "seam", "notes.json")
	return notestore.New(notestore.Config{File: path}), path
}

func id(t *testing.T, c byte) change.ObjectID {
	t.Helper()
	i, err := change.ParseObjectID(strings.Repeat(string(c), 40))
	require.NoError(t, err)
	return i
}

func TestABookReadsBackAsWritten(t *testing.T) {
	s, path := newStore(t)
	at := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	want := note.Book{
		Notes: []note.Note{{
			ID: "n1", Kind: note.Finding, Comparison: "HEAD..worktree", Text: "a\nb", State: note.Open, Created: at, Batch: "b1",
			Anchor: note.Anchor{
				Path: "a.go", Origin: note.Origin{Kind: change.KindCommit, Commit: id(t, '1')}, Blob: id(t, '2'),
				Start: 2, End: 3, Lines: []string{"x", "y"}, Before: []string{"w"}, After: []string{}, Copies: 1, Ordinal: 1,
			},
			Claim:   &note.Claim{ID: "c1", Session: "s1", At: at},
			Replies: []note.Reply{{ID: "r1", Author: "claude", Claim: "c1", Text: "because", At: at}},
		}},
		Sessions: []note.Session{{ID: "s1", Client: "claude", PID: 42, Started: at}},
		Batches:  []note.Batch{{ID: "b1", Session: "s1", Notes: []note.ID{"n1"}, At: at}},
	}

	require.NoError(t, s.Change(t.Context(), func(b *note.Book) error {
		assert.Empty(t, b.Notes, "a missing file is an empty book")
		*b = want
		return nil
	}))

	got, err := s.Load(t.Context())
	require.NoError(t, err)
	assert.Equal(t, want, got)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"version": 1`)
}

func TestAnEmptyBookStillWritesTheWholeFormat(t *testing.T) {
	s, path := newStore(t)
	require.NoError(t, s.Change(t.Context(), func(*note.Book) error { return nil }))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, `{"version": 1, "notes": [], "sessions": [], "batches": []}`, string(data))
}

func TestAFileNotReadIsNeverWritten(t *testing.T) {
	for name, tc := range map[string]struct {
		content string
		err     error
	}{
		"damaged":       {`{"version": 1, "notes": [`, note.ErrNotesDamaged},
		"newer version": {`{"version": 2, "notes": []}`, note.ErrNotesVersion},
		"newer state":   {`{"version": 1, "notes": [{"id": "n", "kind": "question", "state": "taken", "anchor": {"origin": {"kind": "worktree"}}}]}`, note.ErrNotesVersion},
		"bad blob":      {`{"version": 1, "notes": [{"id": "n", "kind": "question", "state": "open", "anchor": {"blob": "xyz", "origin": {"kind": "worktree"}}}]}`, note.ErrNotesDamaged},
	} {
		t.Run(name, func(t *testing.T) {
			s, path := newStore(t)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
			require.NoError(t, os.WriteFile(path, []byte(tc.content), 0o600))

			_, err := s.Load(t.Context())
			require.ErrorIs(t, err, tc.err)
			err = s.Change(t.Context(), func(*note.Book) error {
				t.Fatal("changed a book not read")
				return nil
			})
			require.ErrorIs(t, err, tc.err)

			data, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, tc.content, string(data), "the file stays as it was")
		})
	}
}

func TestAChangeThatFailsWritesNothing(t *testing.T) {
	s, path := newStore(t)
	boom := errors.New("boom")
	require.ErrorIs(t, s.Change(t.Context(), func(*note.Book) error { return boom }), boom)
	_, err := os.Stat(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestChangesOfTwoStoresAreNotLost(t *testing.T) {
	a, path := newStore(t)
	b := notestore.New(notestore.Config{File: path})
	var wg sync.WaitGroup
	for i := range 20 {
		s := a
		if i%2 == 1 {
			s = b
		}
		wg.Go(func() {
			assert.NoError(t, s.Change(context.Background(), func(bk *note.Book) error {
				return bk.Add(note.Draft{Kind: note.Question, Text: "q", Anchor: note.Anchor{Origin: note.Origin{Kind: change.KindIndex}}}, note.ID(rune('a'+i)), time.Time{})
			}))
		})
	}
	wg.Wait()
	got, err := a.Load(t.Context())
	require.NoError(t, err)
	assert.Len(t, got.Notes, 20)
}

func TestALockHeldElsewhereEndsWithTheContext(t *testing.T) {
	s, _ := newStore(t)
	held := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	defer wg.Wait()
	wg.Go(func() {
		assert.NoError(t, s.Change(context.Background(), func(*note.Book) error {
			close(held)
			<-release
			return nil
		}))
	})
	<-held
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	err := s.Change(ctx, func(*note.Book) error { return nil })
	require.ErrorIs(t, err, context.DeadlineExceeded)
	close(release)
}

func TestNoBookOutsideARepository(t *testing.T) {
	s := notestore.New(notestore.Config{})
	_, err := s.Load(t.Context())
	require.ErrorIs(t, err, note.ErrNoBook)
	require.ErrorIs(t, s.Change(t.Context(), func(*note.Book) error { return nil }), note.ErrNoBook)
}
