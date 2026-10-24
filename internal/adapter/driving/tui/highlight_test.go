package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/syntax"
	"seam/internal/domain/ui"
)

type run struct {
	end  int
	kind syntax.Kind
}

// tokens builds the tokens of a side, one slice of runs per line.
func tokens(t *testing.T, lines ...[]run) syntax.Tokens {
	t.Helper()
	var b syntax.Builder
	for _, runs := range lines {
		for _, r := range runs {
			require.NoError(t, b.Add(r.end, r.kind))
		}
		b.EndLine(0)
	}
	return b.Tokens()
}

// highlighted shows "var x = 1" unchanged and "var y = 2" changed to
// "var z = 2", with the keyword and the number marked on both sides.
func highlighted(t *testing.T) *state {
	t.Helper()
	d := diff.Compute(t.Context(), diff.Split("var x = 1\nvar y = 2\n"), diff.Split("var x = 1\nvar z = 2\n"))
	side := tokens(t, []run{{3, syntax.Keyword}, {8, syntax.Plain}, {9, syntax.Number}}, []run{{3, syntax.Keyword}, {8, syntax.Plain}, {9, syntax.Number}})
	return shown(t, files("a.go"), 80, 4, filediff.Snapshot{Diff: d, Highlights: filediff.Highlights{Old: side, New: side}})
}

func TestKindsColourTheirCells(t *testing.T) {
	s := highlighted(t)
	th := newTheme(ui.Dark())

	out := render(s, th)

	lines := strings.Split(out, "\n")
	equal, changed := lines[1], lines[2]
	assert.Contains(t, equal, th.open[textStyle(equalTone, syntax.Keyword, false)]+"var")
	assert.Contains(t, equal, th.open[textStyle(equalTone, syntax.Number, false)]+"1")
	assert.Contains(t, changed, th.open[textStyle(changedTone, syntax.Keyword, false)]+"var")
	// The changed word is plain by kind, in the stronger inline background.
	assert.Contains(t, changed, th.open[textStyle(changedTone, syntax.Plain, true)]+"y")
	assert.Contains(t, changed, th.open[textStyle(changedTone, syntax.Number, false)]+"2")
}

// TestAnInlineSpanKeepsTheKind: a changed keyword is both, stronger and
// coloured as a keyword.
func TestAnInlineSpanKeepsTheKind(t *testing.T) {
	d := diff.Compute(t.Context(), diff.Split("var a\n"), diff.Split("let a\n"))
	side := tokens(t, []run{{3, syntax.Keyword}})
	s := shown(t, files("a.js"), 60, 3, filediff.Snapshot{Diff: d, Highlights: filediff.Highlights{Old: side, New: side}})
	th := newTheme(ui.Dark())

	out := render(s, th)

	assert.Contains(t, out, th.open[textStyle(changedTone, syntax.Keyword, true)]+"var")
	assert.Contains(t, out, th.open[textStyle(changedTone, syntax.Keyword, true)]+"let")
}

func TestScrolledTextKeepsItsKinds(t *testing.T) {
	s := highlighted(t)
	s.shiftH(1) // eight columns: only "1" and "2" are left
	th := newTheme(ui.Dark())

	out := render(s, th)

	assert.Contains(t, out, th.open[textStyle(equalTone, syntax.Number, false)]+"1")
	assert.NotContains(t, out, "var")
}

// TestAStandInIsDimmedWhateverItsKind: a control character in a string is
// replaced and dimmed like any other, not coloured as the string.
func TestAStandInIsDimmedWhateverItsKind(t *testing.T) {
	d := diff.Compute(t.Context(), diff.Split("\"\x07\"\n"), diff.Split("\"\x07\"\n"))
	side := tokens(t, []run{{3, syntax.String}})
	s := shown(t, files("a.go"), 40, 3, filediff.Snapshot{Diff: d, Highlights: filediff.Highlights{Old: side, New: side}})
	th := newTheme(ui.Dark())

	out := render(s, th)

	str := textStyle(equalTone, syntax.String, false)
	assert.Contains(t, out, th.open[str]+`"`+th.close[str]+th.open[paneStyle(equalTone, standInRole)]+"␇")
}

func TestStoppedColoursSaySo(t *testing.T) {
	s := shown(t, files("a.go"), 120, 3, filediff.Snapshot{Notices: []filediff.Notice{
		{Kind: filediff.HighlightStopped, Side: filediff.NewSide, Line: 12},
	}})

	assert.Contains(t, render(s, &theme{}), "new side no syntax colours from line 12")
}
