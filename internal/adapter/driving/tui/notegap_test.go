package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/note"
	"github.com/dturcinovic/seam/internal/domain/ui"
)

func TestAClickOnThePlusPutsTheFieldBetweenTheLines(t *testing.T) {
	m := plusModel(t, splitSettings())
	before := plainLines(&m.s)
	x := m.s.Layout.paneStart(filediff.NewSide)
	m, _ = send(m, tea.MouseMotionMsg{X: x + 10, Y: bodyTop + 4})
	m, _ = send(m, tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: bodyTop + 4})
	require.True(t, m.s.Field.Open)

	lines := plainLines(&m.s)
	h := m.s.Field.H
	assert.Contains(t, lines[bodyTop+4][x:], "●  5", "the line noted is marked")
	assert.Contains(t, lines[bodyTop+5][x:], "└", "the box hangs below it")
	assert.Contains(t, lines[bodyTop+5], "┌ question  finding")
	assert.Contains(t, lines[bodyTop+5], " L5 ─┐")
	assert.Contains(t, lines[bodyTop+6+fieldGap], "Ask about line 5…")
	assert.Equal(t, before[bodyTop+5], lines[bodyTop+5+h], "the next line follows below the box")

	m, _ = typed(m, "esc")
	assert.Equal(t, before, plainLines(&m.s), "the code stands as before")
}

func TestTabSwitchesTheKindAndTheFramesColour(t *testing.T) {
	s := screen(t, 80)
	s.toggleLayout()
	draftAt(s, noteTarget{Side: filediff.NewSide, From: 3, To: 3}, note.Question)
	th := newTheme(ui.Dark())
	frame := func(st style) string { return th.open[st] + "┌" }

	assert.Contains(t, render(s, th), frame(questionFrameStyle))
	assert.Contains(t, render(s, th), th.open[questionChipStyle]+"question")
	assert.Contains(t, plainLines(s)[bodyTop+5+fieldGap], "Ask about line 4…")

	s.noteFieldTab()
	assert.Equal(t, note.Finding, s.Notes.Draft.Kind)
	assert.Contains(t, render(s, th), frame(findingFrameStyle))
	assert.Contains(t, render(s, th), th.open[findingChipStyle]+"finding")
	assert.NotContains(t, render(s, th), frame(questionFrameStyle))
	assert.Contains(t, plainLines(s)[bodyTop+5+fieldGap], "Describe the finding on line 4…")
}

func TestTheBoxGoesWithItsLineWhenTheWheelScrolls(t *testing.T) {
	m, _, _ := startModel(t, "a.go")
	m.s = *screen(t, 80)
	draftAt(&m.s, noteTarget{Side: filediff.NewSide, From: 9, To: 9}, note.Question)
	h := m.s.Field.H

	m, _ = send(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 40, Y: bodyTop + 2})
	require.Equal(t, wheelLines, m.s.top(), "the wheel scrolls with the field open")
	assert.True(t, m.s.Field.Open)

	at := 9 - wheelLines
	lines := plainLines(&m.s)
	assert.Contains(t, lines[bodyTop+at], "● 10")
	assert.Contains(t, lines[bodyTop+at+1], "┌ question")
	assert.Contains(t, lines[bodyTop+at+1+h], " 11 ")
	for i, want := range map[int]int{at: 9, at + 1 + h: 10} {
		got, _, inGap := m.s.bodyRowAt(m.s.gap(), i)
		assert.False(t, inGap)
		assert.Equal(t, want, got, "the body goes on below the box")
	}
	_, row, inGap := m.s.bodyRowAt(m.s.gap(), at+1)
	assert.True(t, inGap)
	assert.Zero(t, row, "the box's top edge")

	m.s.scrollTo(10)
	assert.NotContains(t, strings.Join(plainLines(&m.s), "\n"), "┌ question", "the box went above the top with its line")
}

func TestTheFieldOpensInView(t *testing.T) {
	s := screen(t, 80)
	last := s.lineCount() - 1
	s.scrollTo(last)
	top := s.top()

	draftAt(s, noteTarget{Side: filediff.NewSide, From: 39, To: 39}, note.Question)

	assert.Greater(t, s.top(), top, "the last line moved up to make room")
	lines := plainLines(s)
	assert.Contains(t, lines[bodyTop+s.Layout.shown()-1], "─┘", "the whole box shows")
	assert.Contains(t, lines[bodyTop+s.Layout.shown()-1-s.Field.H], "● 40")
}

func TestTheFieldStandsAtItsFileInTheStream(t *testing.T) {
	s := streamFixture(t)
	first := s.linesOf(2)[1]
	require.False(t, first.Fold)
	line := lineOf(s.snapOf(2).Diff.Rows[first.Rows.Start], filediff.NewSide)
	draftAt(s, noteTarget{File: 2, Side: filediff.NewSide, From: line, To: line}, note.Question)

	g := s.gap()
	require.True(t, g.OK)
	assert.Equal(t, s.bodyIndex(2, 1), g.At)

	draftAt(s, noteTarget{File: 1, Side: filediff.NewSide}, note.Question)
	assert.False(t, s.gap().OK, "a file not loaded has no lines to stand at")
	assert.Equal(t, noteKeys, strings.TrimSpace(plainLines(s)[s.Layout.Height-1]))
}

func TestClosingTheFieldGivesItsRoomBack(t *testing.T) {
	s := screen(t, 80)
	draftAt(s, noteTarget{Side: filediff.NewSide, From: 39, To: 39}, note.Question)
	require.Greater(t, s.top(), s.lineCount()-s.Layout.Body, "scrolled past the last page for the box")

	s.fieldKey(tea.Key{Code: tea.KeyEscape})

	assert.Equal(t, s.lineCount()-s.Layout.Body, s.top(), "the last page is full again")
}

func TestTheFooterNamesTheKeysWhileTheBoxsEdgeIsOffTheScreen(t *testing.T) {
	m, _, _ := startModel(t, "a.go")
	m.s = *screen(t, 80)
	m.s.toggleLayout()
	draftAt(&m.s, noteTarget{Side: filediff.NewSide, From: 30, To: 30}, note.Question)
	assert.Empty(t, footerLine(m))

	m.s.scrollTo(30 + m.s.Field.H - m.s.Layout.shown())
	require.Equal(t, 30+m.s.Field.H-m.s.Layout.shown(), m.s.top())
	assert.Equal(t, noteKeys, footerLine(m), "the bottom edge is below the screen")
}

func TestTheWheelUnderTheFieldOpensNoNotes(t *testing.T) {
	m := plusModel(t, splitSettings())
	m, _ = typed(m, "i", "c")
	require.True(t, m.s.Field.Open)
	caret := m.s.Caret

	for range 4 {
		m, _ = send(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 40, Y: bodyTop + 2})
	}

	require.NotEqual(t, caret, m.s.Caret, "the caret kept to the screen")
	assert.False(t, m.s.Hover.Popup.Open, "no popup covers the field")
}

func TestANarrowBoxKeepsItsCorner(t *testing.T) {
	for _, c := range []struct {
		cols           int
		finding, lines bool
	}{
		{30, true, true},
		{22, false, true},
		{13, false, false},
	} {
		s := fieldScreen(t, c.cols-popupFrame-2*fieldPad)
		draftAt(s, noteTarget{Side: filediff.OldSide, From: 3, To: 3}, note.Question)
		top := boxLines(*s)[0]
		assert.True(t, strings.HasPrefix(top, "┌ question ─") || c.finding, top)
		assert.True(t, strings.HasSuffix(top, "─┐"), top)
		assert.Equal(t, c.finding, strings.Contains(top, "finding"), top)
		assert.Equal(t, c.lines, strings.Contains(top, " L4 "), top)
	}
}
