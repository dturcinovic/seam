package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/hover"
	"seam/internal/domain/ui"
	"seam/internal/textcell"
)

// keyMsg is the key Bubble Tea sends for name.
func keyMsg(name string) tea.KeyPressMsg {
	special := map[string]rune{
		"esc": tea.KeyEscape, "down": tea.KeyDown, "up": tea.KeyUp, "left": tea.KeyLeft, "right": tea.KeyRight,
		"pgdown": tea.KeyPgDown, "tab": tea.KeyTab,
	}
	if c, ok := special[name]; ok {
		return tea.KeyPressMsg{Code: c}
	}
	r := []rune(name)[0]
	return tea.KeyPressMsg{Code: r, Text: name}
}

// caretOnScreen is the screen cell of the caret.
func caretOnScreen(s *state) (x, y int, ok bool) {
	p, ok := s.spot()
	if !ok {
		return 0, 0, false
	}
	return s.caretCell(p)
}

// typed is m after it took the keys named, and the command of the last.
func typed(m model, names ...string) (model, tea.Cmd) {
	var cmd tea.Cmd
	for _, n := range names {
		m, cmd = send(m, keyMsg(n))
	}
	return m, cmd
}

func TestICallsTheCaretOntoTheFirstText(t *testing.T) {
	m, _ := hoverModel(t)

	m, _ = typed(m, "i")

	assert.Equal(t, caret{On: true, Side: filediff.NewSide, Row: 0, Byte: len("\t")}, m.s.Caret)
	text, _ := footer(&m.s)
	assert.Equal(t, "K hover  h/j/k/l move  w/b/e word  o other side  i/esc leave  n/p change  tab/shift+tab file  q quit", text)
	m, _ = typed(m, "i")
	assert.False(t, m.s.Caret.On, "i again takes it away")
}

func TestTheCaretMovesByWords(t *testing.T) {
	m, _ := hoverModel(t)
	m, _ = typed(m, "i")
	// "\tfmt.Println(x)", then the lines "0", "1" and "2".
	var at []caret
	for range 7 {
		m, _ = typed(m, "w")
		at = append(at, m.s.Caret)
	}
	assert.Equal(t, []caret{
		{true, filediff.NewSide, 0, 0, 4}, {true, filediff.NewSide, 0, 0, 5}, {true, filediff.NewSide, 0, 0, 12}, {true, filediff.NewSide, 0, 0, 13}, {true, filediff.NewSide, 0, 0, 14}, {true, filediff.NewSide, 0, 1, 0}, {true, filediff.NewSide, 0, 2, 0},
	}, at, "w: the dot, Println, (, x, ), the next lines")

	m, _ = typed(m, "b", "b", "b")
	assert.Equal(t, caret{true, filediff.NewSide, 0, 0, 13}, m.s.Caret, "b: back over the line's end")
	m, _ = typed(m, "b", "b", "b", "b", "b", "b")
	assert.Equal(t, caret{true, filediff.NewSide, 0, 0, 1}, m.s.Caret, "b stops at the first word")

	m, _ = typed(m, "w", "w", "e")
	assert.Equal(t, caret{true, filediff.NewSide, 0, 0, 11}, m.s.Caret, "e: the end of Println")
	m, _ = typed(m, "e")
	assert.Equal(t, caret{true, filediff.NewSide, 0, 0, 12}, m.s.Caret, "e: a word of one")
}

func TestTheCaretMovesByLinesAndGraphemes(t *testing.T) {
	m, _ := hoverModel(t)
	m, _ = typed(m, "i", "l", "l", "l", "l")
	assert.Equal(t, caret{true, filediff.NewSide, 0, 0, 5}, m.s.Caret, "l: one grapheme on")

	m, _ = typed(m, "j")
	assert.Equal(t, caret{true, filediff.NewSide, 0, 1, 0}, m.s.Caret, "j: the line's end, as it is shorter")
	m, _ = typed(m, "down", "down", "j")
	assert.Equal(t, caret{true, filediff.NewSide, 0, 3, 0}, m.s.Caret, "not into the fold")
	m, _ = typed(m, "k", "up", "k", "k")
	assert.Equal(t, caret{true, filediff.NewSide, 0, 0, 0}, m.s.Caret, "k: the same column, the tab's")
	m, _ = typed(m, "h", "left")
	assert.Equal(t, caret{true, filediff.NewSide, 0, 0, 0}, m.s.Caret, "h stops at the start")
	assert.False(t, m.s.List.Focus || m.s.Layout.List > 0, "the keys were the caret's")
}

func TestTheCaretSkipsRowsWithoutRightText(t *testing.T) {
	d := diff.Compute(t.Context(), diff.Split("a\nb\n\nc\n"), diff.Split("a\n\nc\n"))
	s := shown(t, files("a.go"), 80, 10+chrome, worktreeDoc(filediff.Snapshot{Diff: d}))
	s.startCaret()
	require.Equal(t, caret{true, filediff.NewSide, 0, 0, 0}, s.Caret)

	s.caretUpDown(1)

	assert.Equal(t, "c", diff.Content(d.New.Lines[d.Rows[s.Caret.Row].Right]), "past the deleted line and the empty one")
}

func TestKAsksAtOnceAndOpensThePopupAtTheCaret(t *testing.T) {
	m, f := hoverModel(t)
	m, _ = typed(m, "i", "w", "w")

	m, cmd := typed(m, "K")

	assert.Nil(t, cmd, "no waiting")
	require.Equal(t, 1, f.hover.HoverCallCount())
	assert.Equal(t, hover.Position{Req: 1, Path: "a.go", Version: 1, Line: 0, Byte: 5}, f.hover.HoverArgsForCall(0))
	m = answer(m, text(1, "func fmt.Println(a ...any)"))
	require.True(t, m.s.Hover.Popup.Open)
	x, y, ok := caretOnScreen(&m.s)
	require.True(t, ok)
	assert.Equal(t, y+1, m.s.Hover.Popup.Y, "below the caret")
	assert.Equal(t, m.s.Layout.Width-m.s.Hover.Popup.W, m.s.Hover.Popup.X, "from its column, but within the screen")
	assert.Greater(t, x+m.s.Hover.Popup.W, m.s.Layout.Width)

	m, _ = typed(m, "w")
	assert.False(t, m.s.Hover.Popup.Open, "moving the caret closes it")
	assert.False(t, m.s.Hover.On)
}

func TestKOnABlankAsksNothing(t *testing.T) {
	m, f := hoverModel(t)
	m, _ = typed(m, "i", "h", "K")

	assert.Zero(t, f.hover.HoverCallCount())
	assert.False(t, m.s.Hover.On)
}

func TestEscClosesThePopupThenTheCaretThenQuits(t *testing.T) {
	m, _ := hoverModel(t)
	m, _ = typed(m, "i", "K")
	m = answer(m, text(1, "package fmt"))
	require.True(t, m.s.Hover.Popup.Open)

	m, cmd := typed(m, "esc")
	assert.Nil(t, cmd)
	assert.False(t, m.s.Hover.Popup.Open)
	assert.True(t, m.s.Caret.On)

	m, cmd = typed(m, "esc")
	assert.Nil(t, cmd)
	assert.False(t, m.s.Caret.On)

	_, cmd = typed(m, "esc")
	require.NotNil(t, cmd)
	assert.Equal(t, tea.Quit(), cmd())
}

func TestTheViewFollowsTheCaretAndTheCaretTheView(t *testing.T) {
	m, _ := hoverModel(t)
	m, _ = typed(m, "f", "i")
	for range 30 {
		m, _ = typed(m, "j")
	}
	_, y, ok := caretOnScreen(&m.s)
	require.True(t, ok, "the view scrolled along")
	assert.Equal(t, bodyTop+m.s.Layout.shown()-1, y)

	m, _ = typed(m, "g")

	assert.Zero(t, m.s.Pos.Scroll)
	_, y, ok = caretOnScreen(&m.s)
	require.True(t, ok, "the caret came along")
	assert.Equal(t, bodyTop+m.s.Layout.shown()-1, y, "to the nearest line on the screen")
}

func TestTheViewFollowsTheCaretSideways(t *testing.T) {
	long := strings.Repeat("ab ", 60)
	d := diff.Compute(t.Context(), diff.Split("x\n"), diff.Split(long+"\n"))
	s := shown(t, files("a.go"), 80, 10+chrome, worktreeDoc(filediff.Snapshot{Diff: d}))
	s.startCaret()
	from, to := s.Layout.text(filediff.NewSide)

	for range 50 {
		s.wordForward()
		x, _, ok := caretOnScreen(s)
		require.True(t, ok)
		require.GreaterOrEqual(t, x, from)
		require.Less(t, x, to)
	}
	assert.Positive(t, s.HScroll)
	for range 50 {
		s.wordBack()
	}
	assert.Zero(t, s.HScroll)
}

func TestTheCaretShowsInverted(t *testing.T) {
	m, _ := hoverModel(t)
	m, _ = typed(m, "i", "w", "w")
	th := newTheme(ui.Dark())

	lines := strings.Split(render(&m.s, th), "\n")

	assert.Contains(t, lines[bodyTop], th.open[caretStyle]+"P")
	assert.NotContains(t, lines[bodyTop+1], th.open[caretStyle])
}

func TestAnotherFileTakesTheCaretAway(t *testing.T) {
	m, _ := hoverModel(t)
	m.s.setFiles(files("a.go", "b.go"))
	m, _ = typed(m, "i", "tab")

	assert.False(t, m.s.Caret.On)
}

func TestAReloadKeepsTheCaretOnAGrapheme(t *testing.T) {
	d := diff.Compute(t.Context(), diff.Split("x\n"), diff.Split("日本語\n"))
	s := shown(t, files("a.go"), 80, 10+chrome, worktreeDoc(filediff.Snapshot{Diff: d}))
	s.startCaret()
	s.caretSideways(1)
	require.Equal(t, len("日"), s.Caret.Byte)
	s.Snapshot.Diff = diff.Compute(t.Context(), diff.Split("x\n"), diff.Split("ab日本語\n"))
	s.Caret.Byte = 4

	s.keepCaret()

	assert.Equal(t, 2, s.Caret.Byte, "back to the start of 日")
}

func TestNoCaretWithoutADocument(t *testing.T) {
	d := diff.Compute(t.Context(), diff.Split("a\n"), diff.Split("b\n"))
	s := shown(t, files("a.go"), 80, 10+chrome, filediff.Snapshot{Diff: d})

	s.startCaret()

	assert.False(t, s.Caret.On, "neither side has a document")
}

// TestTheCaretFindsGraphemesThroughTheColumnIndex: on a long line, the
// caret's graphemes and columns are those of a walk from the line's start,
// whatever checkpoint it resumes at.
func TestTheCaretFindsGraphemesThroughTheColumnIndex(t *testing.T) {
	long := strings.Repeat("ab\t日é👨‍👩 ", 20<<10/24)
	d := diff.Compute(t.Context(), diff.Split("x\n"), diff.Split(long+"\n"))
	for _, m := range methods {
		s := shown(t, files("a.go"), 100, 10+chrome, worktreeDoc(filediff.Snapshot{Diff: d, Columns: columnsOf(t, d)}))
		s.Cells = m.method
		require.NotEmpty(t, s.Snapshot.Columns.New.At(0))
		walk := textcell.New(long, m.method)
		for c, ok := walk.Next(); ok; c, ok = walk.Next() {
			require.Equal(t, c, s.spotAt(0, c.End-1).c, "%s at byte %d", m.name, c.End-1)
		}
		s.startCaret()
		p, _ := s.spot()
		for range 2000 {
			s.wordForward()
		}
		q, _ := s.spot()
		assert.Greater(t, q.c.Start, 12<<10, "past several checkpoints")
		assert.Equal(t, q.c, s.spotAtCol(0, q.c.Col).c)
		for range 2000 {
			s.wordBack()
		}
		r, _ := s.spot()
		assert.Equal(t, p.c, r.c, "back at the start")
	}
}

func TestTheCaretStaysInTheColumnsShown(t *testing.T) {
	long := strings.Repeat("ab ", 60)
	d := diff.Compute(t.Context(), diff.Split("x\n"), diff.Split(long+"\n"))
	s := shown(t, files("a.go"), 80, 10+chrome, worktreeDoc(filediff.Snapshot{Diff: d}))
	s.shiftH(5)
	s.startCaret()
	require.Equal(t, s.HScroll, s.Caret.Byte, "i starts at the first column shown")

	s.shiftH(5)
	s.keepCaret()

	x, _, ok := caretOnScreen(s)
	require.True(t, ok)
	from, to := s.Layout.text(filediff.NewSide)
	assert.GreaterOrEqual(t, x, from)
	assert.Less(t, x, to)
	s.shiftH(-10)
	s.keepCaret()
	x, _, _ = caretOnScreen(s)
	assert.Less(t, x, to, "and back")
}

func TestACaretWithNowhereToGoGoes(t *testing.T) {
	d := diff.Compute(t.Context(), diff.Split("x\n"), diff.Split("a\n"))
	s := shown(t, files("a.go"), 80, 10+chrome, worktreeDoc(filediff.Snapshot{Diff: d}))
	s.startCaret()
	require.True(t, s.Caret.On)
	gone := diff.Compute(t.Context(), diff.Split("x\n"), diff.Split(""))
	s.Snapshot.Diff = gone
	s.relayout()

	s.keepCaret()

	assert.False(t, s.Caret.On, "a deleted line takes no caret, so its keys are free again")
}

func TestACaretOnAScreenWithoutBody(t *testing.T) {
	m, _ := hoverModel(t)
	m, _ = typed(m, "i")

	m, _ = send(m, tea.WindowSizeMsg{Width: 80, Height: chrome})
	m, _ = typed(m, "j", "w", "K")

	assert.True(t, m.s.Caret.On)
}
