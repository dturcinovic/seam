package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/diff"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/textcell"
)

// worktreeDoc gives a snapshot's right side the worktree's document; the
// left side, without one, has no hovers.
func worktreeDoc(snap filediff.Snapshot) filediff.Snapshot {
	snap.RightDoc = hover.Document{Path: "a.go", Version: 1}
	return snap
}

// renderedAt is the grapheme the screen shows at column x of line, as a
// terminal counting by m reads the plain rendering.
func renderedAt(line string, x int, m textcell.Method) string {
	cells := textcell.New(line, m)
	for c, ok := cells.Next(); ok; c, ok = cells.Next() {
		if c.Col <= x && x < c.Col+c.Width {
			return line[c.Start:c.End]
		}
	}
	return ""
}

// screenHits hit-tests every column of the right pane's text on screen
// line i of s, whose right side is line, and returns how many had a
// target: each starts with the grapheme drawn there, and only blanks have
// none.
func screenHits(t *testing.T, s *state, screen []string, i int, line string) int {
	t.Helper()
	from, to := s.Layout.text(filediff.NewSide)
	hits := 0
	for x := from; x < to; x++ {
		shownThere := renderedAt(screen[bodyTop+i], x, s.Cells)
		got, ok := s.targetAt(x, bodyTop+i, hoverTarget{Line: -1})
		if !ok {
			assert.Equal(t, " ", shownThere, "line %d column %d", i, x)
			continue
		}
		hits++
		assert.True(t, strings.HasPrefix(line[got.Byte:], shownThere), "line %d column %d: byte %d, %q shown", i, x, got.Byte, shownThere)
	}
	return hits
}

// TestATargetIsTheGraphemeOnScreen renders the unicode lines and hit-tests
// every cell of the right pane's text: a target starts with the grapheme
// drawn there, both cells of a wide one included, and only blanks have
// none.
func TestATargetIsTheGraphemeOnScreen(t *testing.T) {
	text := diff.Compute(t.Context(),
		diff.Split("cafe\nfamily 👨‍👩‍👧 x\n日本語のテキスト\na\tb\n日\tb\n👨‍👩‍👧\tb\n"),
		diff.Split("café\nfamily 👨‍👩‍👧 y\n日本語のテキスト\na\tb\n日\tb\n👨‍👩‍👧\tb\n"))
	for _, m := range methods {
		for _, scroll := range []int{0, 1, 3} {
			t.Run(fmt.Sprintf("%s scroll %d", m.name, scroll), func(t *testing.T) {
				s := shown(t, files("名前.go"), 60, 9, worktreeDoc(filediff.Snapshot{Diff: text}))
				s.toggleMode()
				s.Cells, s.HScroll = m.method, scroll
				screen := plainLines(s)
				hits := 0
				for i := range s.Lines {
					right := s.Snapshot.Diff.Rows[s.Lines[i].Rows.Start].Right
					hits += screenHits(t, s, screen, i, diff.Content(s.Snapshot.Diff.New.Lines[right]))
				}
				assert.Positive(t, hits)
			})
		}
	}
}

// TestATargetBesideTheFileList hit-tests a screen with the file list open
// and the file scrolled down, which moves the pane and its lines.
func TestATargetBesideTheFileList(t *testing.T) {
	lines := strings.Repeat("cafe\u0301 日本語\n👨\u200d👩\u200d👧\tb\n", 6)
	d := diff.Compute(t.Context(), diff.Split(lines), diff.Split(lines+"x\n"))
	s := shown(t, listFiles(), 140, 9, worktreeDoc(filediff.Snapshot{Diff: d}))
	s.toggleMode()
	s.scrollTo(5)
	require.Positive(t, s.Layout.List)
	require.Positive(t, s.Pos.Scroll)
	screen := plainLines(s)

	hits := 0
	for i := range s.Layout.shown() {
		right := s.Snapshot.Diff.Rows[s.Lines[s.Pos.Scroll+i].Rows.Start].Right
		hits += screenHits(t, s, screen, i, diff.Content(s.Snapshot.Diff.New.Lines[right]))
	}

	assert.Positive(t, hits)
}

func TestATargetHasItsIdentifier(t *testing.T) {
	line := "\tfmt.Println(cafe\u0301_1, $v)"
	d := diff.Compute(t.Context(), diff.Split("x\n"), diff.Split(line+"\n"))
	s := shown(t, files("a.go"), 80, 5, worktreeDoc(filediff.Snapshot{Diff: d}))
	from, _ := s.Layout.text(filediff.NewSide)
	tests := []struct {
		col  int
		want string
	}{
		{4, "fmt"}, {6, "fmt"}, {7, "."}, {8, "Println"}, {14, "Println"}, {15, "("},
		{16, "café_1"}, {21, "café_1"}, {22, ","}, {24, "$v"}, {26, ")"},
	}
	for _, tt := range tests {
		got, ok := s.targetAt(from+tt.col, bodyTop, hoverTarget{Line: -1})
		require.True(t, ok, tt.col)
		assert.Equal(t, tt.want, line[got.Word.Start:got.Word.End], "column %d", tt.col)
	}
}

func TestNoTargetOutsideTheTextOfASideWithDocument(t *testing.T) {
	d := diff.Compute(t.Context(), diff.Split(numbered(20)), diff.Split(numbered(20, 10)))
	s := shown(t, files("a.go"), 80, 12, worktreeDoc(filediff.Snapshot{Diff: d}))
	from, to := s.Layout.text(filediff.NewSide)
	fold := 0
	require.True(t, s.Lines[fold].Fold)
	changed := 4
	require.Equal(t, diff.Change, s.Snapshot.Diff.Rows[s.Lines[changed].Rows.Start].Kind)
	_, ok := s.targetAt(from, bodyTop+changed, hoverTarget{Line: -1})
	require.True(t, ok, "the first column of the text")

	for _, c := range []struct {
		name string
		x, y int
	}{
		{"the header", from, 0},
		{"the fold", from, bodyTop + fold},
		{"the gutter", from - 1, bodyTop + changed},
		{"the separator", s.Layout.List + s.Layout.Left.Width, bodyTop + changed},
		{"the left pane without document", s.Layout.List + 6, bodyTop + changed},
		{"past the line's end", to - 1, bodyTop + changed},
		{"the scrollbar", s.Layout.Width - 1, bodyTop + changed},
		{"the footer", from, s.Layout.Height - 1},
	} {
		_, ok := s.targetAt(c.x, c.y, hoverTarget{Line: -1})
		assert.False(t, ok, c.name)
	}
}

func TestNoTargetWhereTheServerHasNothingToSay(t *testing.T) {
	tests := []struct {
		name     string
		old, new string
		snap     func(filediff.Snapshot) filediff.Snapshot
		line     int
	}{
		{"a side without document", "a\n", "b\n", func(s filediff.Snapshot) filediff.Snapshot { return s }, 0},
		{"a row without right line", "a\nb\n", "a\n", worktreeDoc, 1},
		{"a line past the limit", "x\n", strings.Repeat("a", hover.MaxLine+1) + "\n", worktreeDoc, 0},
	}
	for _, tt := range tests {
		d := diff.Compute(t.Context(), diff.Split(tt.old), diff.Split(tt.new))
		s := shown(t, files("a.go"), 80, 5, tt.snap(filediff.Snapshot{Diff: d}))
		from, _ := s.Layout.text(filediff.NewSide)

		_, ok := s.targetAt(from, bodyTop+tt.line, hoverTarget{Line: -1})

		assert.False(t, ok, tt.name)
	}
}

// TestALongLineIsHitWhereItIsDrawn hit-tests a 52 KiB line near its end,
// where both the renderer and the hit-test start from a checkpoint.
func TestALongLineIsHitWhereItIsDrawn(t *testing.T) {
	long := strings.Repeat("ab\t日é👨‍👩 ", 60<<10/24)
	require.Less(t, len(long), hover.MaxLine)
	d := diff.Compute(t.Context(), diff.Split(long+"\n"), diff.Split(long+"!\n"))
	for _, m := range methods {
		s := shown(t, files("min.js"), 100, 3, worktreeDoc(filediff.Snapshot{Diff: d, Columns: columnsOf(t, d)}))
		s.Cells, s.HScroll = m.method, textWidth(long, m.method)-40
		require.NotEmpty(t, s.Snapshot.Columns.New.At(0))
		assert.Positive(t, screenHits(t, s, plainLines(s), 0, long+"!"), m.name)
	}
}
