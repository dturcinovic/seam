package tui

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/change"
	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/syntax"
	"seam/internal/domain/ui"
	"seam/internal/textcell"
)

// methods are both ways a terminal counts columns, with how x/ansi measures
// a line under each.
var methods = []struct {
	name   string
	method textcell.Method
	width  func(string) int
}{
	{"codepoints", textcell.Codepoints, ansi.StringWidthWc},
	{"graphemes", textcell.Graphemes, ansi.StringWidth},
}

// plainLines renders s without colours, split into lines.
func plainLines(s *state) []string {
	return strings.Split(render(s, &theme{}), "\n")
}

func TestANewlineInAPathShowsAsNewline(t *testing.T) {
	s := shown(t, files("a\nb.go"), 80, 5, filediff.Snapshot{})

	assert.Contains(t, render(s, &theme{}), "a␤b.go")
}

func TestAFailedLoadSaysSo(t *testing.T) {
	s := newState(change.Changeset{Files: files("a.go")}, ui.DefaultSettings())
	s.resize(80, 5)
	r, _ := s.open(0, false)
	s.apply(filediff.Update{Gen: r.Gen, Err: errors.New("boom")})

	out := render(&s, &theme{})

	assert.Contains(t, out, "failed")
	assert.NotContains(t, out, "loading")
	assert.Contains(t, out, "error: boom")
}

// evil holds what must not reach the terminal: OSC 52 (clipboard), a CSI
// that clears the screen, BEL, a C1 CSI, a bidi override and an invalid
// byte.
const evil = "x\x1b]52;c;aGk=\a\x1b[2J\u009b31m\u202e\xff"

// TestNoForeignControlReachesTheTerminal renders a file whose name, lines
// and error hold escape sequences. Without colours nothing below 0x20 but
// the line breaks comes out; with them, only the theme's own sequences.
func TestNoForeignControlReachesTheTerminal(t *testing.T) {
	file := change.ChangedFile{Status: change.Modified, OldPath: "n" + evil, NewPath: "n" + evil}
	d := diff.Compute(t.Context(), diff.Split("a"+evil+"\n"), diff.Split("b"+evil+"\n"))
	s := shown(t, []change.ChangedFile{file}, 120, 10, filediff.Snapshot{Diff: d})
	s.Err = errors.New("git: " + evil)

	for _, m := range methods {
		s.Cells = m.method
		noControls(t, render(s, &theme{}))
		noControls(t, withoutTheme(render(s, newTheme(ui.Dark()))))
	}
	th := newTheme(ui.Dark())
	header, _, _ := strings.Cut(render(s, th), "\n")
	assert.Contains(t, header, th.open[headerStandIn]+"␛", "stand-ins are dimmed in the header too")
}

// withoutTheme removes every sequence the theme writes.
func withoutTheme(out string) string {
	th := newTheme(ui.Dark())
	var pairs []string
	for i := range styles {
		for _, seq := range []string{th.open[i], th.close[i]} {
			if seq != "" {
				pairs = append(pairs, seq, "")
			}
		}
	}
	return strings.NewReplacer(pairs...).Replace(out)
}

func noControls(t *testing.T, out string) {
	t.Helper()
	for i := range len(out) {
		require.False(t, (out[i] < 0x20 && out[i] != '\n') || out[i] == 0x7f, "control byte %#x at %d in %q", out[i], i, out)
	}
	require.True(t, utf8.ValidString(out))
}

func TestRender(t *testing.T) {
	s := loaded(t)

	lines := plainLines(s)

	require.Len(t, lines, s.Layout.Height)
	assert.Contains(t, lines[0], "1/3  M a.go  hunks  2 differences")
	assert.Contains(t, lines[1], "⋯ 7 unchanged lines")
	assert.Contains(t, lines[5], "11 ~ 10")
	assert.Contains(t, lines[5], "11 ~ changed 10")
}

func TestRenderWhileLoading(t *testing.T) {
	s := newState(change.Changeset{Files: files("a.go")}, ui.DefaultSettings())
	s.resize(40, 5)
	s.open(0, false)

	assert.Contains(t, render(&s, &theme{}), "loading…")
	empty := newState(change.Changeset{}, ui.DefaultSettings())
	empty.resize(40, 5)
	assert.Contains(t, render(&empty, &theme{}), "nothing to compare")
}

func TestNotices(t *testing.T) {
	s := shown(t, files("a.bin"), 100, 5, filediff.Snapshot{Notices: []filediff.Notice{
		{Kind: filediff.TooLarge, Side: filediff.OldSide, Size: 9},
		{Kind: filediff.Binary, Side: filediff.NewSide},
		{Kind: filediff.Simplified},
	}})

	out := render(s, &theme{})

	assert.Contains(t, out, "old side too large (9 bytes)  new side binary  diff simplified")
	assert.Contains(t, out, "not shown as text")
}

// TestEveryLineIsAsWideAsTheScreen measures each rendered line, colours
// included, as the renderer does under each method, for the fixtures of
// the golden files and narrow windows down to one column.
func TestEveryLineIsAsWideAsTheScreen(t *testing.T) {
	for _, m := range methods {
		for _, f := range fixtures(t) {
			for _, width := range []int{f.state.Layout.Width, 80, 23, 9, 2, 1} {
				s := *f.state
				s.Cells = m.method
				s.resize(width, s.Layout.Height)
				for i, line := range strings.Split(render(&s, newTheme(ui.Dark())), "\n") {
					require.Equal(t, width, m.width(line), "%s %s width %d line %d: %q", m.name, f.name, width, i, line)
				}
			}
		}
	}
}

// TestAWideCharacterCutByAnEdgeIsBlank: scrolled into the middle of 日, its
// visible half is a blank in the line's colour, and so is the half the
// right edge cuts.
func TestAWideCharacterCutByAnEdgeIsBlank(t *testing.T) {
	d := diff.Compute(t.Context(), diff.Split("日本\n"), diff.Split("日本\n"))
	// Panes of 9 columns, a gutter of 5 and 4 of text, and the scrollbar.
	s := shown(t, files("a.txt"), 20, 3, filediff.Snapshot{Diff: d})

	s.HScroll = 1
	assert.Equal(t, " 1    本 │ 1    本  ", plainLines(s)[1])
	s.HScroll = 0
	s.resize(18, 3)
	assert.Equal(t, " 1   日 │ 1   日  ", plainLines(s)[1], "half of 本 at the right edge")
}

// TestALongLineCostsWhatIsVisible renders a file of one 8 MiB line: only the
// visible window is read, so a frame allocates a small part of the line.
func TestALongLineCostsWhatIsVisible(t *testing.T) {
	long := strings.Repeat("x", 8<<20)
	d := diff.Compute(t.Context(), diff.Split(long+"\n"), diff.Split(long+"y\n"))
	s := shown(t, files("min.js"), 120, 30, filediff.Snapshot{Diff: d})
	s.shiftH(100)

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	out := render(s, newTheme(ui.Dark()))
	runtime.ReadMemStats(&after)

	assert.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(1<<20))
	assert.Contains(t, ansi.Strip(out), "~ xxx")
}

// TestTheColumnIndexStartsWhereTheWalkWould renders a 52 KiB line of mixed
// widths near its end, once from checkpoints and once walking from the
// start: both show the same.
func TestTheColumnIndexStartsWhereTheWalkWould(t *testing.T) {
	long := strings.Repeat("ab\t日e\u0301👨\u200d👩 ", 60<<10/24)
	d := diff.Compute(t.Context(), diff.Split(long+"\n"), diff.Split(long+"!\n"))
	for _, m := range methods {
		indexed := shown(t, files("min.js"), 100, 3, filediff.Snapshot{Diff: d, Columns: columnsOf(t, d)})
		walked := shown(t, files("min.js"), 100, 3, filediff.Snapshot{Diff: d})
		for _, s := range []*state{indexed, walked} {
			s.Cells = m.method
			s.HScroll = textWidth(long, m.method) - 40
		}
		require.NotEmpty(t, indexed.Snapshot.Columns.New.At(0))
		out := render(indexed, newTheme(ui.Dark()))
		require.Contains(t, ansi.Strip(out), "ab", "the screen shows text")

		assert.Equal(t, render(walked, newTheme(ui.Dark())), out, m.name)
	}
}

// columnsOf indexes the single line of both sides with the checkpoints the
// loader takes, every 4 KiB.
func columnsOf(t *testing.T, d diff.FileDiff) filediff.Columns {
	side := func(s diff.Side) filediff.ColumnIndex {
		found, err := textcell.Checkpoints(t.Context(), diff.Content(s.Lines[0]), 4<<10)
		require.NoError(t, err)
		points := make([]filediff.Checkpoint, len(found))
		for i, p := range found {
			points[i] = filediff.Checkpoint(p)
		}
		return filediff.ColumnIndex{Lines: []filediff.LineCheckpoints{{Line: 0, Points: points}}}
	}
	return filediff.Columns{Old: side(d.Old), New: side(d.New)}
}

func TestTheRendererCountsGraphemesOnceTheTerminalKnowsMode2027(t *testing.T) {
	tests := []struct {
		value ansi.ModeSetting
		want  textcell.Method
	}{
		{ansi.ModeNotRecognized, textcell.Codepoints},
		{ansi.ModeSet, textcell.Graphemes},
		{ansi.ModeReset, textcell.Graphemes},
		{ansi.ModePermanentlySet, textcell.Graphemes},
		{ansi.ModePermanentlyReset, textcell.Codepoints},
	}
	for _, tt := range tests {
		m, _, _ := startModel(t, "a.go")

		next, _ := m.Update(tea.ModeReportMsg{Mode: ansi.ModeUnicodeCore, Value: tt.value})

		assert.Equal(t, tt.want, next.(model).s.Cells, "value %d", tt.value)
	}
	m, _, _ := startModel(t, "a.go")
	next, _ := m.Update(tea.ModeReportMsg{Mode: ansi.ModeSynchronizedOutput, Value: ansi.ModeSet})
	assert.Equal(t, textcell.Codepoints, next.(model).s.Cells, "another mode")
}

// TestTheThemeIsLipGloss: each style switches on and off exactly as Lip
// Gloss renders it, and the changed lines are coloured at all.
func TestTheThemeIsLipGloss(t *testing.T) {
	th := newTheme(ui.Dark())
	for st, ls := range lipglossStyles(colours(ui.Dark())) {
		assert.Equal(t, ls.Render("x"), th.open[st]+"x"+th.close[st])
	}
	for _, st := range []style{textStyle(changedTone, syntax.Plain, false), textStyle(changedTone, syntax.Plain, true), textStyle(deletedTone, syntax.Plain, false)} {
		assert.NotEmpty(t, th.open[st])
	}
	assert.NotEqual(t, th.open[textStyle(changedTone, syntax.Plain, false)], th.open[textStyle(changedTone, syntax.Plain, true)])
	assert.NotEqual(t, th.open[textStyle(equalTone, syntax.Plain, false)], th.open[textStyle(equalTone, syntax.Keyword, false)],
		"kinds are coloured on equal lines too")
}

func TestATinyScreenGetsNoMoreLinesThanItHas(t *testing.T) {
	s := loaded(t)
	for height := 1; height <= 4; height++ {
		s.resize(80, height)

		assert.Len(t, plainLines(s), height)
	}
}

// TestASpanInsideAGraphemeMarksIt: the word diff splits a skin tone off its
// emoji, so the only change starts within the grapheme.
func TestASpanInsideAGraphemeMarksIt(t *testing.T) {
	d := diff.Compute(t.Context(), diff.Split("ok 👍\U0001F3FB\n"), diff.Split("ok 👍\U0001F3FD\n"))
	s := shown(t, files("a.txt"), 40, 3, filediff.Snapshot{Diff: d})
	left, _ := d.InlineAt(0)
	require.NotEmpty(t, left)
	require.Greater(t, left[0].Start, len("ok "), "the span starts after the thumb")

	th := newTheme(ui.Dark())
	assert.Contains(t, render(s, th), th.open[textStyle(changedTone, syntax.Plain, true)]+"👍")
}
