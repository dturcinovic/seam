package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/note"
)

// linePopup is noteModel with the popup of a line holding notes a and b.
func linePopup(t *testing.T) (model, noteFakes) {
	t.Helper()
	m, f := placedAt(t, spotAt("a", note.Question, note.Exact, 2, 2), spotAt("b", note.Finding, note.Exact, 2, 2))
	m, _ = typed(m, "i", "j")
	require.True(t, m.s.Hover.Popup.Open)
	return m, f
}

func TestDThenYDeletesTheFirstNoteOfALine(t *testing.T) {
	m, f := linePopup(t)

	m, _ = typed(m, "D")
	assert.Zero(t, f.deleter.DeleteCallCount(), "D only asks")
	text, _ := footer(&m.s)
	assert.Equal(t, "delete question “text of a”? y deletes, any other key keeps it", text, "names the note of a line's several")

	m, _ = typed(m, "y")
	require.Equal(t, 1, f.deleter.DeleteCallCount())
	assert.Equal(t, note.ID("a"), f.deleter.DeleteArgsForCall(0))
	assert.Equal(t, "note deleted", m.s.Note)
	assert.True(t, m.s.Hover.Popup.Open, "a line's popup follows the book")
}

func TestAnyOtherKeyKeepsTheNote(t *testing.T) {
	// D above all: a key held down repeats it, and that must not confirm.
	for _, key := range []string{"D", "esc", "x", "j", "q", "Y"} {
		t.Run(key, func(t *testing.T) {
			m, f := linePopup(t)

			m, cmd := typed(m, "D", key)

			assert.Zero(t, f.deleter.DeleteCallCount())
			assert.Zero(t, f.writer.ResolveCallCount(), "the answer does nothing else")
			assert.Nil(t, cmd, "not even quit")
			assert.True(t, m.s.Hover.Popup.Open)
			assert.Equal(t, "note kept", m.s.Note)
			m, _ = typed(m, "y")
			assert.Zero(t, f.deleter.DeleteCallCount(), "asked no more")
		})
	}
}

func TestAClickKeepsTheNote(t *testing.T) {
	overview := func(t *testing.T) (model, noteFakes) {
		t.Helper()
		m, f := noteModel(t, headToWorktree(t))
		return overviewOf(t, m, f, bookNote("a", "a.go", worktreeOrigin(), 1, 1)), f
	}
	for name, start := range map[string]func(*testing.T) (model, noteFakes){"popup": linePopup, "overview": overview} {
		t.Run(name, func(t *testing.T) {
			m, f := start(t)
			m, _ = typed(m, "D")

			m, _ = send(m, tea.MouseClickMsg{Button: tea.MouseLeft, X: 1, Y: 1})
			_, _ = typed(m, "y")

			assert.Zero(t, f.deleter.DeleteCallCount(), "asked no more")
		})
	}
}

func TestDThenYOnTheOverviewDeletesTheNoteChosen(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = overviewOf(t, m, f, bookNote("a", "a.go", worktreeOrigin(), 1, 1), bookNote("b", "a.go", worktreeOrigin(), 2, 2))

	m, _ = typed(m, "j", "D", "y")

	require.Equal(t, 1, f.deleter.DeleteCallCount())
	assert.Equal(t, note.ID("b"), f.deleter.DeleteArgsForCall(0))
	assert.True(t, m.s.Notes.Overview.Open)
}

func TestDeletingTheNoteOfItsPopupClosesIt(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	// An index note no side shows, so that enter opens its popup.
	m = overviewOf(t, m, f, bookNote("a", "a.go", note.Origin{Kind: change.KindIndex}, 1, 1))
	m, _ = typed(m, "enter")
	require.True(t, m.s.Hover.Popup.Open)

	m, _ = typed(m, "D", "y")

	require.Equal(t, 1, f.deleter.DeleteCallCount())
	assert.False(t, m.s.Hover.Popup.Open)
}

func TestADeleteThatFailsSaysWhy(t *testing.T) {
	m, f := linePopup(t)
	f.deleter.DeleteReturns(4)
	m, _ = typed(m, "D", "y")

	m = notesCome(m, f, note.Update{Kind: note.Done, Op: 4, Err: fmt.Errorf("read notes: %w", note.ErrNotesDamaged)})

	assert.Equal(t, "notes: read notes: notes file damaged", m.s.Note)
}

func TestTheQuestionNamesTheNoteByItsFirstLine(t *testing.T) {
	assert.Equal(t, "short", deletionName("short"))
	assert.Equal(t, "first…", deletionName("first\nsecond"))
	assert.Equal(t, strings.Repeat("ä", deletionNameRunes)+"…", deletionName(strings.Repeat("ä", deletionNameRunes+1)))
}

func TestTheMouseRestsWhileAsked(t *testing.T) {
	m, f := linePopup(t)
	m, _ = typed(m, "D")
	before := m.s.Hover.Popup

	for _, msg := range []tea.Msg{tea.MouseMotionMsg{X: 70, Y: 10}, tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 70, Y: 10}, hoverDue{Seq: m.s.Hover.Seq}} {
		m, _ = send(m, msg)
	}

	assert.Equal(t, before, m.s.Hover.Popup)
	_, _ = typed(m, "y")
	assert.Equal(t, 1, f.deleter.DeleteCallCount(), "the question still stands")
}

func TestTheQuestionShowsOverAnError(t *testing.T) {
	m, _ := linePopup(t)
	m.s.Err = errors.New("watch failed")

	m, _ = typed(m, "D")

	text, _ := footer(&m.s)
	assert.Contains(t, text, "delete question")
}

func TestANoteGoneElsewhereEndsTheQuestion(t *testing.T) {
	m, f := linePopup(t)
	head := func(id note.ID) note.Head {
		return note.Head{ID: id, Path: "a.go", Origin: worktreeOrigin(), State: note.Open}
	}
	m = notesCome(m, f, note.Update{Kind: note.StoreStatus, Heads: []note.Head{head("a"), head("b")}, Live: true})
	m, _ = typed(m, "D")

	m = notesCome(m, f, note.Update{Kind: note.StoreStatus, Err: fmt.Errorf("read notes: %w", note.ErrNotesDamaged)})
	assert.True(t, m.s.Notes.Delete.asking(), "a book not read says nothing of the note")

	m = notesCome(m, f, note.Update{Kind: note.StoreStatus, Heads: []note.Head{head("b")}, Live: true})
	assert.Equal(t, "note gone, nothing deleted", m.s.Note)
	_, _ = typed(m, "y")
	assert.Zero(t, f.deleter.DeleteCallCount())
}
