package tui

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/hover"
	"seam/internal/domain/ui"
	"seam/internal/domain/view"
)

// splitSettings are the default settings with the sides side by side at
// any width, as the tests written before the unified view expect them.
func splitSettings() ui.Settings {
	s := ui.DefaultSettings()
	s.Layout = ui.SplitLayout
	return s
}

func changedSnapshot(t *testing.T) filediff.Snapshot {
	t.Helper()
	return filediff.Snapshot{Diff: diff.Compute(t.Context(), diff.Split(numbered(60)), diff.Split(numbered(60, 10, 40)))}
}

func TestTheDiffsWidthDecidesTheLayout(t *testing.T) {
	s := shownWith(t, ui.DefaultSettings(), files("a.go"), 120, 20, changedSnapshot(t))
	assert.False(t, s.Layout.Unified, "120 columns without the list")

	s.resize(119, 20)
	assert.True(t, s.Layout.Unified)
	assert.Contains(t, header(s), "hunks unified")
	assert.True(t, hasNewHalves(s.Lines), "the lines are laid out anew")

	s.resize(130, 20)
	assert.False(t, s.Layout.Unified)
	assert.False(t, hasNewHalves(s.Lines))

	s.toggleList()
	assert.True(t, s.Layout.Unified, "the file list narrows the diff")
	s.toggleList()
	assert.False(t, s.Layout.Unified)
}

func hasNewHalves(lines []view.Line) bool {
	return slices.ContainsFunc(lines, func(l view.Line) bool { return l.Half == view.NewHalf })
}

func TestNoAutomaticLayoutWithoutAWidth(t *testing.T) {
	set := ui.DefaultSettings()
	set.SplitMinWidth = 0
	s := shownWith(t, set, files("a.go"), 20, 20, changedSnapshot(t))
	assert.False(t, s.Layout.Unified)

	set.Layout = ui.UnifiedLayout
	s = shownWith(t, set, files("a.go"), 300, 20, changedSnapshot(t))
	assert.True(t, s.Layout.Unified)
}

func TestUTurnsTheSidesForGood(t *testing.T) {
	m, _ := hoverModel(t)
	m.s.LayoutSetting = ui.AutoLayout
	m, _ = send(m, tea.WindowSizeMsg{Width: 200, Height: 24})
	require.False(t, m.s.Layout.Unified)

	m, _ = typed(m, "u")
	assert.True(t, m.s.Layout.Unified)
	m, _ = send(m, tea.WindowSizeMsg{Width: 300, Height: 24})
	assert.True(t, m.s.Layout.Unified, "no longer automatic")

	m, _ = typed(m, "u")
	assert.False(t, m.s.Layout.Unified)
	m, _ = send(m, tea.WindowSizeMsg{Width: 60, Height: 24})
	assert.False(t, m.s.Layout.Unified)

	text, _ := footer(&m.s)
	assert.Contains(t, text, "f full/hunks  u unified/split  ")
}

func TestTheTopHalfStaysAcrossLayouts(t *testing.T) {
	s := shownWith(t, ui.DefaultSettings(), files("a.go"), 100, 8, changedSnapshot(t))
	s.toggleMode()
	require.True(t, s.Layout.Unified)
	// Rows 0 to 9, then the old and the new half of row 10.
	s.scrollTo(11)
	top := s.Lines[s.Pos.Scroll]
	require.Equal(t, diff.RowRange{Start: 10, End: 11}, top.Rows)
	require.Equal(t, view.NewHalf, top.Half)

	s.toggleLayout()
	assert.Equal(t, 10, s.Pos.Scroll, "side by side, the row is one line")
	s.toggleLayout()
	assert.Equal(t, 11, s.Pos.Scroll, "the new half again, not the old one")
}

// unifiedHoverModel is hoverModel turned unified: the old half of the
// changed first row on top, then its new half with codeLine.
func unifiedHoverModel(t *testing.T) (model, hoverFakes) {
	t.Helper()
	m, f := hoverModel(t)
	m, _ = typed(m, "u")
	require.True(t, m.s.Layout.Unified)
	require.Equal(t, view.NewHalf, m.s.Lines[1].Half)
	return m, f
}

func TestAHoverOnANewHalf(t *testing.T) {
	m, f := unifiedHoverModel(t)
	from, _ := m.s.Layout.text(filediff.NewSide)

	_, cmd := send(m, tea.MouseMotionMsg{X: from + 5, Y: bodyTop})
	assert.Nil(t, cmd, "the old side has no document")

	m, cmd = send(m, tea.MouseMotionMsg{X: from + 5, Y: bodyTop + 1})
	require.NotNil(t, cmd)
	send(m, hoverDue{Seq: m.s.Hover.Seq})
	require.Equal(t, 1, f.hover.HoverCallCount())
	assert.Equal(t, hover.Position{Req: 1, Path: "a.go", Version: 1, Line: 0, Byte: 2}, f.hover.HoverArgsForCall(0))
}

func TestTheCaretFollowsTheHalves(t *testing.T) {
	m, f := unifiedHoverModel(t)

	m, _ = typed(m, "i")
	assert.Equal(t, caret{On: true, Side: filediff.NewSide, Row: 0, Byte: len("\t")}, m.s.Caret, "past the old half without a document")
	x, y, ok := caretOnScreen(&m.s)
	require.True(t, ok)
	from, _ := m.s.Layout.text(filediff.NewSide)
	assert.Equal(t, [2]int{from + 4, bodyTop + 1}, [2]int{x, y})
	assert.Contains(t, plainLines(&m.s)[bodyTop+1], "+")

	m, _ = typed(m, "o")
	_, y, _ = caretOnScreen(&m.s)
	assert.Equal(t, bodyTop+1, y, "the old half has no document")

	m, _ = typed(m, "K")
	require.Equal(t, 1, f.hover.HoverCallCount())
	assert.Equal(t, hover.Position{Req: 1, Path: "a.go", Version: 1, Line: 0, Byte: 1}, f.hover.HoverArgsForCall(0))

	m, _ = typed(m, "j")
	assert.Equal(t, caret{On: true, Side: filediff.NewSide, Row: 1, Byte: 0}, m.s.Caret, "the whole row below")
}

func TestOtherSideSwitchesHalves(t *testing.T) {
	d := diff.Compute(t.Context(), diff.Split("old one\n"), diff.Split("new one\n"))
	s := shownWith(t, ui.DefaultSettings(), files("a.go"), 80, 10, filediff.Snapshot{
		Diff:    d,
		LeftDoc: hover.Document{Path: "a.go"}, RightDoc: hover.Document{Path: "a.go"},
	})
	require.True(t, s.Layout.Unified)
	s.startCaret()
	require.Equal(t, caret{On: true, Side: filediff.OldSide, Row: 0, Byte: 0}, s.Caret)
	s.wordForward()

	s.otherSide()
	assert.Equal(t, caret{On: true, Side: filediff.NewSide, Row: 0, Byte: len("new ")}, s.Caret)
	s.otherSide()
	assert.Equal(t, caret{On: true, Side: filediff.OldSide, Row: 0, Byte: len("old ")}, s.Caret)
}
