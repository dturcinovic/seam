package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/view"
)

// The state of loaded shows, from the top: the fold of rows 0 to 6, rows 7
// to 13 around the first change, the fold of rows 14 to 36 and so on.

func TestClickingAFoldUnfoldsIt(t *testing.T) {
	t.Run("on top", func(t *testing.T) {
		s := loaded(t)
		require.True(t, s.Lines[0].Fold)

		s.unfoldAt(10, bodyTop)

		assert.Equal(t, 0, view.LineForRow(s.Lines, 0))
		assert.False(t, s.Lines[0].Fold)
		assert.Equal(t, view.Pos{}, s.Pos, "row 0 is on top")
	})
	t.Run("further down", func(t *testing.T) {
		s := loaded(t)
		s.scrollBy(2)
		pos, fold := s.Pos, view.LineForRow(s.Lines, 20)
		require.True(t, s.Lines[fold].Fold)

		s.unfoldAt(60, bodyTop+fold-s.Pos.Scroll)

		assert.False(t, s.Lines[view.LineForRow(s.Lines, 20)].Fold)
		assert.Equal(t, 14, view.RowAt(s.Lines, fold), "the fold's first row stands where it was")
		assert.Equal(t, pos, s.Pos, "the view stays")
	})
}

func TestAClickElsewhereChangesNothing(t *testing.T) {
	ref := loaded(t)
	foldY := bodyTop + view.LineForRow(ref.Lines, 20)
	tests := []struct {
		name string
		x, y int
	}{
		{"a row", 10, bodyTop + 1},
		{"the header", 10, 0},
		{"the footer", 10, bodyTop + ref.Layout.Body},
		{"the scrollbar beside a fold", ref.Layout.panes(), foldY},
		{"left of the screen", -1, foldY},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := loaded(t)

			s.unfoldAt(tt.x, tt.y)

			assert.Equal(t, ref.Lines, s.Lines)
			assert.Equal(t, ref.Pos, s.Pos)
		})
	}
}

func TestUnfoldingMovesTheMarks(t *testing.T) {
	s := loaded(t)
	before := s.Marks

	s.unfoldAt(10, bodyTop)

	assert.NotEqual(t, before, s.Marks, "seven more lines above the first change")
	assert.Equal(t, changeMarks(s.Snapshot.Diff, s.Lines, s.Layout.Body), s.Marks)
}

func TestTheFoldsReturnWithTheNextLayout(t *testing.T) {
	s := loaded(t)
	folded := len(s.Lines)
	s.unfoldAt(10, bodyTop)
	require.Greater(t, len(s.Lines), folded)

	s.toggleMode()
	s.toggleMode()

	assert.Len(t, s.Lines, folded)
}

func TestTheWheel(t *testing.T) {
	tests := []struct {
		name          string
		button        tea.MouseButton
		mod           tea.KeyMod
		scroll, shift int
	}{
		{"down", tea.MouseWheelDown, 0, wheelLines, 0},
		{"up", tea.MouseWheelUp, 0, 0, 0},
		{"shift down goes right", tea.MouseWheelDown, tea.ModShift, 0, hStep},
		{"right", tea.MouseWheelRight, 0, 0, hStep},
		{"left", tea.MouseWheelLeft, 0, 0, 0},
		{"shift up goes left", tea.MouseWheelUp, tea.ModShift, 0, 0},
		{"ctrl down still scrolls", tea.MouseWheelDown, tea.ModCtrl, wheelLines, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := model{s: *loaded(t)}

			next, _ := m.Update(tea.MouseWheelMsg{Button: tt.button, Mod: tt.mod})

			s := next.(model).s
			assert.Equal(t, tt.scroll, s.Pos.Scroll)
			assert.Equal(t, tt.shift, s.HScroll)
		})
	}
	t.Run("back again", func(t *testing.T) {
		m := model{s: *loaded(t)}
		for _, b := range []tea.MouseButton{tea.MouseWheelDown, tea.MouseWheelDown, tea.MouseWheelUp, tea.MouseWheelRight, tea.MouseWheelRight, tea.MouseWheelLeft} {
			next, _ := m.Update(tea.MouseWheelMsg{Button: b})
			m = next.(model)
		}
		assert.Equal(t, wheelLines, m.s.Pos.Scroll)
		assert.Equal(t, hStep, m.s.HScroll)
	})
}

func TestOnlyTheLeftButtonUnfolds(t *testing.T) {
	for _, b := range []tea.MouseButton{tea.MouseLeft, tea.MouseRight, tea.MouseMiddle} {
		m := model{s: *loaded(t)}
		folded := len(m.s.Lines)

		next, _ := m.Update(tea.MouseClickMsg{Button: b, X: 10, Y: bodyTop})

		assert.Equal(t, b == tea.MouseLeft, len(next.(model).s.Lines) > folded, b.String())
	}
}

func TestTheViewCatchesClicksAndTheWheel(t *testing.T) {
	m := model{s: *loaded(t), theme: &theme{}}

	assert.Equal(t, tea.MouseModeCellMotion, m.View().MouseMode)
}
