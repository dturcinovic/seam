package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/note"
	"github.com/dturcinovic/seam/internal/domain/ui"
)

// plusModel is noteModel with a question at line 2 of the worktree side
// and a finding at line 5 of HEAD's; unified, line 4 is a change's two
// halves.
func plusModel(t *testing.T, set ui.Settings) model {
	t.Helper()
	m, f := noteModelWith(t, headToWorktree(t), set)
	return notesCome(m, f,
		note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{spotAt("q", note.Question, note.Exact, 2, 2)}},
		note.Update{Kind: note.Placed, Key: headSide(t), Spots: []note.Spot{spotAt("f", note.Finding, note.Exact, 5, 5)}})
}

func TestTheMouseIsOfferedAPlusOnLinesWithoutAMark(t *testing.T) {
	split, unified := plusModel(t, splitSettings()), plusModel(t, ui.DefaultSettings())
	right := split.s.Layout.paneStart(filediff.NewSide)
	old, wt := filediff.OldSide, filediff.NewSide
	for _, c := range []struct {
		name string
		m    model
		x, y int
		want noteLine
		ok   bool
	}{
		{"left text", split, 10, 0, noteLine{Side: old, Line: 0}, true},
		{"left gutter", split, 0, 0, noteLine{Side: old, Line: 0}, true},
		{"right text", split, right + 10, 0, noteLine{Side: wt, Line: 0}, true},
		{"the separator", split, right - 1, 0, noteLine{}, false},
		{"a line with a mark", split, right + 10, 1, noteLine{}, false},
		{"the other side of it", split, 10, 1, noteLine{Side: old, Line: 1}, true},
		{"a whole row, unified", unified, 10, 0, noteLine{Side: wt, Line: 0}, true},
		{"a whole row with the old side's mark", unified, 10, 5, noteLine{}, false},
		{"a whole row with the new side's mark", unified, 10, 1, noteLine{}, false},
		{"the old half of a change", unified, 10, 3, noteLine{Side: old, Line: 3}, true},
		{"the new half of a change", unified, 10, 4, noteLine{Side: wt, Line: 3}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			l, ok := c.m.s.plusAt(c.x, bodyTop+c.y)
			assert.Equal(t, c.ok, ok)
			assert.Equal(t, c.want, l)
		})
	}
}

func TestNoPlusOnASideThatTakesNoNotes(t *testing.T) {
	m, _ := noteModel(t, change.Comparison{Left: change.File("/tmp/a"), Right: change.File("/tmp/b")})
	_, ok := m.s.plusAt(10, bodyTop)
	assert.False(t, ok)
}

func TestThePlusShowsInTheGutterAndGoesWithTheMouse(t *testing.T) {
	m := plusModel(t, splitSettings())
	right := m.s.Layout.paneStart(filediff.NewSide)
	mark := func(m model, x int) string { return string([]rune(plainLines(&m.s)[bodyTop])[x]) }

	m, _ = send(m, tea.MouseMotionMsg{X: right + 10, Y: bodyTop})
	assert.Equal(t, "+", mark(m, right))
	assert.Equal(t, " ", mark(m, 0), "only on the side under the mouse")

	m, _ = send(m, tea.MouseMotionMsg{X: 10, Y: bodyTop})
	assert.Equal(t, " ", mark(m, right))
	assert.Equal(t, "+", mark(m, 0))

	for _, away := range []tea.Msg{
		tea.MouseMotionMsg{X: 10, Y: 0},
		tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 10, Y: bodyTop},
		tea.KeyPressMsg{Code: 'c', Text: "c"},
	} {
		m, _ := send(m, tea.MouseMotionMsg{X: 10, Y: bodyTop})
		require.True(t, m.s.Hover.PlusOn)
		m, _ = send(m, away)
		assert.False(t, m.s.Hover.PlusOn, "%T", away)
	}
}

func TestAClickOnThePlusOpensTheFieldForItsLine(t *testing.T) {
	for _, c := range []struct {
		name   string
		set    ui.Settings
		side   filediff.Which
		y      int
		origin note.Origin
		start  int
	}{
		{"the worktree side", splitSettings(), filediff.NewSide, 4, note.Origin{Kind: change.KindWorktree}, 5},
		{"the commit side", splitSettings(), filediff.OldSide, 6, note.Origin{Kind: change.KindCommit, Commit: objectID(t, "1")}, 7},
		{"a whole row, unified", ui.DefaultSettings(), filediff.NewSide, 1 + 6, note.Origin{Kind: change.KindWorktree}, 7},
	} {
		t.Run(c.name, func(t *testing.T) {
			m, f := noteModelWith(t, headToWorktree(t), c.set)
			x := m.s.Layout.paneStart(c.side)
			m, _ = send(m, tea.MouseMotionMsg{X: x + 10, Y: bodyTop + c.y})
			m, _ = send(m, tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: bodyTop + c.y})
			require.True(t, m.s.Field.Open)
			assert.False(t, m.s.Hover.PlusOn, "the field covers it")

			m, _ = typed(m, "w", "h", "y", "enter")
			require.Equal(t, 1, f.writer.AddCallCount())
			a := f.writer.AddArgsForCall(0).Anchor
			assert.Equal(t, c.origin, a.Origin)
			assert.Equal(t, c.start, a.Start)
			assert.Equal(t, c.start, a.End)
		})
	}
}

func TestAClickBesideThePlusOpensNoField(t *testing.T) {
	m, _ := noteModel(t, headToWorktree(t))
	right := m.s.Layout.paneStart(filediff.NewSide)
	m, _ = send(m, tea.MouseMotionMsg{X: right + 10, Y: bodyTop})
	m, _ = send(m, tea.MouseClickMsg{Button: tea.MouseLeft, X: right + 1, Y: bodyTop})
	assert.False(t, m.s.Field.Open)
}

func TestAPlusNotShownTakesNoClick(t *testing.T) {
	m := plusModel(t, splitSettings())
	right := m.s.Layout.paneStart(filediff.NewSide)
	click := tea.MouseClickMsg{Button: tea.MouseLeft, X: right, Y: bodyTop}

	m, _ = send(m, click)
	assert.False(t, m.s.Field.Open, "no motion came, no plus showed")

	m, _ = send(m, tea.MouseMotionMsg{X: right + 10, Y: bodyTop})
	m, _ = send(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: right + 10, Y: bodyTop})
	m, _ = send(m, click)
	assert.False(t, m.s.Field.Open, "the wheel took the plus away")
}

func TestNoPlusUnderThePopupOrOnAMark(t *testing.T) {
	m := plusModel(t, splitSettings())
	right := m.s.Layout.paneStart(filediff.NewSide)
	m, _ = send(m, tea.MouseMotionMsg{X: right, Y: bodyTop + 1})
	require.True(t, m.s.Hover.Popup.Open, "the mark opened its notes")
	assert.False(t, m.s.Hover.PlusOn)

	p := m.s.Hover.Popup
	m, _ = send(m, tea.MouseMotionMsg{X: p.X + 1, Y: p.Y + 1})
	assert.False(t, m.s.Hover.PlusOn, "the mouse is on the popup, not on a line")
}

func TestAClickOnThePlusKeepsTheLinesMarked(t *testing.T) {
	m := plusModel(t, splitSettings())
	m, _ = typed(m, "i", "v")
	require.True(t, m.s.Notes.Mark.On)
	right := m.s.Layout.paneStart(filediff.NewSide)
	m, _ = send(m, tea.MouseMotionMsg{X: right + 10, Y: bodyTop + 6})
	m, _ = send(m, tea.MouseClickMsg{Button: tea.MouseLeft, X: right, Y: bodyTop + 6})
	require.True(t, m.s.Field.Open)
	assert.True(t, m.s.Notes.Mark.On)
}
