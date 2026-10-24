package tui

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/change"
	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
)

func TestReplacedCharacters(t *testing.T) {
	tests := map[string]string{
		"plain":          "plain",
		"tab\there":      "tab    here",
		"esc\x1b[31m":    "esc␛[31m",
		"bell\a":         "bell␇",
		"del\x7f":        "del␡",
		"c1\u009b":       `c1\x9B`,
		"bad\xff":        `bad\xFF`,
		"bidi\u202ecba":  "bidi<U+202E>cba",
		"isolate\u2066x": "isolate<U+2066>x",
		"größe und 日本":   "größe und 日本",
	}
	for in, want := range tests {
		var b strings.Builder
		writeCells(&b, in, 0, 100)
		assert.Equal(t, want, b.String(), "%q", in)
	}
}

func TestANewlineInAPathShowsAsNewline(t *testing.T) {
	s := shown(t, files("a\nb.go"), 80, 5, filediff.Snapshot{})

	assert.Contains(t, render(s), "a\u2424b.go")
}

func TestAFailedLoadSaysSo(t *testing.T) {
	s := newState(change.Changeset{Files: files("a.go")})
	s.resize(80, 5)
	r, _ := s.open(0, false)
	s.apply(filediff.Update{Gen: r.Gen, Err: errors.New("boom")})

	out := render(&s)

	assert.Contains(t, out, "failed")
	assert.NotContains(t, out, "loading")
	assert.Contains(t, out, "error: boom")
}

// TestNoForeignControlReachesTheTerminal renders a file whose name and lines
// hold escape sequences: nothing below 0x20 but the line breaks comes out.
func TestNoForeignControlReachesTheTerminal(t *testing.T) {
	evil := "x\x1b]52;c;aGk=\a\x1b[2J\u009b31m\u202e\xff"
	file := change.ChangedFile{Status: change.Modified, OldPath: "n" + evil, NewPath: "n" + evil}
	d := diff.Compute(t.Context(), diff.Split("a"+evil+"\n"), diff.Split("b"+evil+"\n"))
	s := shown(t, []change.ChangedFile{file}, 120, 10, filediff.Snapshot{Diff: d})
	s.Err = assert.AnError

	out := render(s)

	for i := range len(out) {
		require.False(t, out[i] < 0x20 && out[i] != '\n', "control byte %#x at %d in %q", out[i], i, out)
	}
	assert.True(t, utf8.ValidString(out))
}

func TestRender(t *testing.T) {
	s := loaded(t)

	lines := strings.Split(render(s), "\n")

	require.Len(t, lines, s.Height)
	assert.Contains(t, lines[0], "1/3  M a.go  hunks  2 differences")
	assert.Contains(t, lines[1], "⋯ 7 unchanged lines")
	assert.Contains(t, strings.Join(lines, "\n"), "~ 10")
	assert.Contains(t, strings.Join(lines, "\n"), "~ changed 10")
	for i, l := range lines {
		assert.Equal(t, s.Width, utf8.RuneCountInString(l), "line %d is as wide as the screen", i)
	}
}

func TestRenderWhileLoading(t *testing.T) {
	s := newState(change.Changeset{Files: files("a.go")})
	s.resize(40, 5)
	s.open(0, false)

	assert.Contains(t, render(&s), "loading…")
	empty := newState(change.Changeset{})
	empty.resize(40, 5)
	assert.Contains(t, render(&empty), "nothing to compare")
}

func TestNotices(t *testing.T) {
	s := shown(t, files("a.bin"), 100, 5, filediff.Snapshot{Notices: []filediff.Notice{
		{Kind: filediff.TooLarge, Side: filediff.OldSide, Size: 9},
		{Kind: filediff.Binary, Side: filediff.NewSide},
		{Kind: filediff.Simplified},
	}})

	out := render(s)

	assert.Contains(t, out, "old side too large (9 bytes)  new side binary  diff simplified")
	assert.Contains(t, out, "not shown as text")
}

func TestWriteCellsCountsReplacedColumns(t *testing.T) {
	var b strings.Builder
	// a, four spaces for the tab, b, ␁, c: columns 2 to 5 are "   b".
	writeCells(&b, "a\tb\x01c", 2, 4)

	assert.Equal(t, "   b", b.String())
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
	out := render(s)
	runtime.ReadMemStats(&after)

	assert.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(1<<20))
	assert.Contains(t, out, "~ xxx")
}
