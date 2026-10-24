package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/hover"
	"seam/internal/textcell"
)

// code is an answer of one block of code, as the adapter gives it.
func code(s string) hover.Content {
	return hover.Content{Blocks: []hover.Block{{Kind: hover.Code, Code: strings.Trim(s, "\n")}}}
}

// texts are the texts of p's lines.
func texts(p popup) []string {
	var ts []string
	for _, l := range p.Lines {
		ts = append(ts, l.Text)
	}
	return ts
}

func screen(t *testing.T, width int) *state {
	t.Helper()
	d := diff.Compute(t.Context(), diff.Split(numbered(40)), diff.Split(numbered(40, 20)))
	s := shown(t, files("a.go"), width, 24, worktreeDoc(filediff.Snapshot{Diff: d}))
	s.toggleMode()
	return s
}

func TestThePopupSitsBelowTheMouse(t *testing.T) {
	s := screen(t, 80)

	p := s.openPopup(code("func f()\n"), 10, 5)

	assert.Equal(t, []string{"func f()"}, texts(p))
	p.Lines = nil
	assert.Equal(t, popup{Open: true, X: 10, Y: 6, W: 10, H: 3}, p)
}

func TestThePopupGoesAboveWithoutRoomBelow(t *testing.T) {
	s := screen(t, 80)
	last := bodyTop + s.Layout.shown() - 1

	p := s.openPopup(code("a\nb"), 10, last)

	assert.Equal(t, last-4, p.Y)
	assert.Equal(t, 4, p.H)
}

func TestThePopupStaysOnTheScreen(t *testing.T) {
	s := screen(t, 80)

	p := s.openPopup(code(strings.Repeat("word ", 40)+"\n"+strings.Repeat("line\n", 40)), 75, 3)

	assert.Equal(t, 0, p.X, "as wide as allowed, so from the left edge")
	assert.Equal(t, popupMaxWidth, p.W)
	assert.Equal(t, 24*popupMaxPercent/100, p.H)
	for _, l := range texts(p) {
		assert.LessOrEqual(t, textWidth(l, s.Cells), p.W-popupFrame, l)
	}
	narrow := screen(t, 30).openPopup(code("func fmt.Println(a ...any) (n int, err error)"), 20, 3)
	assert.Equal(t, 30, narrow.W)
	assert.Equal(t, 0, narrow.X)
}

func TestNothingToShowOpensNoPopup(t *testing.T) {
	s := screen(t, 80)

	assert.False(t, s.openPopup(code("\n\n"), 10, 5).Open)
	assert.False(t, screen(t, 2).openPopup(code("x"), 0, 5).Open, "no room for the frame")
}

func TestCodeIsWrappedAtGraphemes(t *testing.T) {
	lines, widest := popupText(code("日本語のテキスト\tx\n\nab"), 5, textcell.Graphemes)

	assert.Equal(t, []string{"日本", "語の", "テキ", "スト ", "   x", "", "ab"}, texts(popup{Lines: lines}))
	assert.Equal(t, 5, widest)
}

// TestThePopupCoversItsCellsOnly renders a popup whose edges cut wide
// characters under it: every line keeps the screen's width, the code
// beside the popup stays in its columns, and its text arrives.
func TestThePopupCoversItsCellsOnly(t *testing.T) {
	wide := strings.Repeat("日本語のテキスト", 4)
	d := diff.Compute(t.Context(), diff.Split(wide+"\n"+wide+"\n"+wide+"\n"), diff.Split(wide+"\n"+wide+"!\n"+wide+"\n"))
	for _, m := range methods {
		// Both edges fall on either cell of a wide character somewhere.
		for x := 44; x < 48; x++ {
			s := shown(t, files("a.go"), 80, 10, worktreeDoc(filediff.Snapshot{Diff: d}))
			s.toggleMode()
			s.Cells = m.method
			before := plainLines(s)
			s.Hover.Popup = s.openPopup(code("func fmt.Println"), x, bodyTop)
			require.True(t, s.Hover.Popup.Open)
			p := s.Hover.Popup

			after := plainLines(s)

			for i, l := range after {
				assert.Equal(t, s.Layout.Width, m.width(l), "%s x %d line %d: %q", m.name, x, i, l)
			}
			assert.Contains(t, after[p.Y+1], "│func fmt.Println│", m.name)
			assert.Equal(t, before[bodyTop], after[bodyTop], "the line of the mouse")
			// A wide character cut by the edge leaves a blank.
			right := strings.Trim(ansiCutLeft(after[p.Y+1], p.X+p.W, m.method), " ")
			assert.True(t, strings.HasSuffix(strings.TrimRight(before[p.Y+1], " "), right), "%s x %d: %q after the popup", m.name, x, right)
		}
	}
}

func ansiCutLeft(s string, n int, m textcell.Method) string {
	cells := textcell.New(s, m)
	for c, ok := cells.Next(); ok; c, ok = cells.Next() {
		if c.Col >= n {
			return s[c.Start:]
		}
	}
	return ""
}

func TestAPopupTextReachesTheTerminalOnlyAsText(t *testing.T) {
	s := screen(t, 80)
	s.Hover.Popup = s.openPopup(code("type T"+evil), 10, 5)
	require.True(t, s.Hover.Popup.Open)

	for _, m := range methods {
		s.Cells = m.method
		noControls(t, render(s, &theme{}))
		noControls(t, withoutTheme(render(s, newTheme())))
	}
}
