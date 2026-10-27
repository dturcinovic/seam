package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/note"
	"github.com/dturcinovic/seam/internal/domain/ui"
)

// placedAt is noteModel with spots placed in the worktree side.
func placedAt(t *testing.T, spots ...note.Spot) (model, noteFakes) {
	t.Helper()
	m, f := noteModel(t, headToWorktree(t))
	return notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: spots}), f
}

func TestANoteArrivingAboveTheTopKeepsTheTopLine(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m.s.scrollTo(20)
	// Without the scrollbar, whose thumb moves.
	topLine := func() string { return strings.TrimRight(plainLines(&m.s)[bodyTop], " ━") }
	top := topLine()

	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{
		spotAt("a", note.Question, note.Exact, 3, 3), spotAt("b", note.Finding, note.Exact, 5, 5),
	}})

	assert.Equal(t, 22, m.s.top(), "two note lines came above it")
	assert.Equal(t, top, topLine())
}

func TestCHidesTheNoteLinesAndKeepsTheMarks(t *testing.T) {
	m, _ := placedAt(t, spotAt("a", note.Question, note.Exact, 3, 3))
	shown := strings.Join(plainLines(&m.s), "\n")
	require.Contains(t, shown, "?   text of a")

	m, _ = typed(m, "C")
	hidden := strings.Join(plainLines(&m.s), "\n")
	assert.NotContains(t, hidden, "text of a")
	assert.Contains(t, hidden, "◆  3", "the mark stays")

	m, _ = typed(m, "C")
	assert.Equal(t, shown, strings.Join(plainLines(&m.s), "\n"))
}

func TestCKeepsANoteLineOnTop(t *testing.T) {
	m, _ := placedAt(t, spotAt("a", note.Question, note.Exact, 30, 30), spotAt("b", note.Question, note.Exact, 30, 30))
	m.s.scrollTo(31)
	require.Contains(t, plainLines(&m.s)[bodyTop], "text of b")

	m, _ = typed(m, "C")
	assert.Contains(t, plainLines(&m.s)[bodyTop], " 30 ", "without its line, the line it hangs at")
	m, _ = typed(m, "C")
	assert.Contains(t, plainLines(&m.s)[bodyTop], "text of b", "and back")
}

func TestAResolvedNoteTakesItsLineAlong(t *testing.T) {
	m, f := placedAt(t, spotAt("a", note.Question, note.Exact, 3, 3))
	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide()})
	shown := strings.Join(plainLines(&m.s), "\n")
	assert.NotContains(t, shown, "text of a")
	assert.NotContains(t, shown, "◆")
}

func TestEveryNoteAtALineHasALine(t *testing.T) {
	long := spotAt("long", note.Question, note.Exact, 3, 3)
	long.Note.Text = strings.Repeat("word ", 30) + "\nsecond line"
	m, _ := placedAt(t, long, spotAt("f", note.Finding, note.Likely, 3, 3))
	lines := plainLines(&m.s)

	first := lines[bodyTop+3]
	assert.Contains(t, first, "│    ?   word word")
	assert.True(t, strings.HasSuffix(strings.TrimRight(first, " "), "…"), "cut to the pane: %q", first)
	assert.NotContains(t, first, "second line", "its first line only")
	assert.Contains(t, lines[bodyTop+4], "!   text of f")

	th := newTheme(ui.Dark())
	rendered := strings.Split(render(&m.s, th), "\n")
	assert.Contains(t, rendered[bodyTop+4], th.open[findingDimTextStyle]+"text of f", "likely placed, dim")
	assert.Contains(t, rendered[bodyTop+3], th.open[questionTextStyle]+"word")
}

func TestTheCaretAndTheMouseGoPastNoteLines(t *testing.T) {
	m, _ := placedAt(t, spotAt("a", note.Question, note.Exact, 3, 3))
	m, _ = typed(m, "i", "j", "j")
	require.Equal(t, 2, m.s.Caret.Row)
	m, _ = typed(m, "j")
	assert.Equal(t, 3, m.s.Caret.Row, "past the note's line to the next row")
	m, _ = typed(m, "k")
	assert.Equal(t, 2, m.s.Caret.Row)
	m, _ = typed(m, "i")

	right := m.s.Layout.paneStart(1)
	_, ok := m.s.targetAt(right+10, bodyTop+3, hoverTarget{})
	assert.False(t, ok, "no hover on a note's line")
	_, ok = m.s.plusAt(right+1, bodyTop+3)
	assert.False(t, ok, "no plus")
	m, _ = send(m, tea.MouseMotionMsg{X: right + 10, Y: bodyTop + 3})
	assert.False(t, m.s.Hover.PlusOn)
}

func TestANoteInAFoldShowsBelowItUntilItOpens(t *testing.T) {
	m, _ := placedAt(t, spotAt("a", note.Question, note.Exact, 30, 30))
	m.s.toggleMode()
	lines := plainLines(&m.s)
	fold := -1
	for i, l := range lines {
		if strings.Contains(l, "unchanged lines") && fold < 0 && i > bodyTop+2 {
			fold = i
		}
	}
	require.Positive(t, fold)
	require.Contains(t, lines[fold+1], "text of a", "the hunk view shows it below its fold")

	m.s.unfoldLine(m.s.top() + fold - bodyTop)
	lines = plainLines(&m.s)
	for i, l := range lines {
		if strings.Contains(l, "text of a") {
			assert.Contains(t, lines[i-1], " 30 ", "below its line once the fold opened")
		}
	}
}

func TestTheFieldStandsBelowTheNotesOfItsLine(t *testing.T) {
	m, _ := placedAt(t, spotAt("a", note.Question, note.Exact, 3, 3))
	right := m.s.Layout.paneStart(1)
	m, _ = send(m, tea.MouseMotionMsg{X: right + 10, Y: bodyTop + 2})
	m, _ = send(m, tea.MouseClickMsg{Button: tea.MouseLeft, X: right, Y: bodyTop + 2})
	require.True(t, m.s.Field.Open)

	lines := plainLines(&m.s)
	assert.Contains(t, lines[bodyTop+2], "●  3")
	assert.Contains(t, lines[bodyTop+3], "text of a")
	assert.Contains(t, lines[bodyTop+4], "┌ question")
}

func TestTheSameNotesAgainMoveNoLine(t *testing.T) {
	spots := []note.Spot{spotAt("a", note.Question, note.Exact, 3, 3)}
	m, f := placedAt(t, spots...)
	laid := m.s.laid()

	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: spots})

	assert.Equal(t, laid, m.s.laid(), "where m goes on from still holds")
}
