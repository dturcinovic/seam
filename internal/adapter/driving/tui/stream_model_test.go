package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/app/port/in/infakes"
	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
)

// streamModel is the stream over a.go and b.go, each the right side
// "\tfmt.Println(x)" after a line of two changed, served by gopls, on a
// screen of 80 by 24 with both files loaded.
func streamModel(t *testing.T) (model, *infakes.FakeHoverer) {
	t.Helper()
	m, fl, _ := startModel(t, "a.go", "b.go")
	m.p.Docs.(*infakes.FakeDocumentTracker).TrackReturns(gopls(), true)
	hv := m.p.Hover.(*infakes.FakeHoverer)
	hv.HoverReturns(hover.Sent)
	m, _ = send(m, fileDiffsReady{})
	require.Equal(t, 1, fl.LoadCallCount())

	m, _ = typed(m, "s")

	sl := m.p.Stream.(*infakes.FakeStreamLoader)
	require.Equal(t, 1, sl.WantCallCount(), "s asks for the window")
	text := "x\n" + codeLine + "\n"
	d := diff.Compute(t.Context(), diff.Split("x\ny\n"), diff.Split(text))
	var us []filediff.Update
	for _, r := range sl.WantArgsForCall(0) {
		us = append(us, filediff.Update{Gen: r.Gen, File: r.File, Snapshot: filediff.Snapshot{
			File: r.File, Diff: d, RightDoc: hover.Document{Path: r.File.NewPath, Lang: lang.Go, Version: 1, Text: text},
		}})
	}
	require.Len(t, us, 2)
	sl.DrainReturns(us)
	m, _ = send(m, streamReady{})
	return m, hv
}

func TestTheStreamTellsTheServersOfEachFile(t *testing.T) {
	m, _ := streamModel(t)

	docs := m.p.Docs.(*infakes.FakeDocumentTracker)
	var paths []string
	for i := range docs.TrackCallCount() {
		paths = append(paths, docs.TrackArgsForCall(i).Path)
	}
	assert.Contains(t, paths, "b.go")
	assert.Equal(t, gopls(), m.s.instanceOf(1, filediff.NewSide))
}

func TestAHoverAsksAboutTheFileUnderTheMouse(t *testing.T) {
	m, hv := streamModel(t)
	// b.go's code line, below a.go and b.go's header.
	line := m.s.bodyIndex(1, 1)
	_, l, ok := m.s.rowLine(line)
	require.True(t, ok)
	require.Equal(t, 1, l.Rows.Start, "the second row, the code line")
	from, _ := m.s.layoutOf(1).text(filediff.NewSide)

	m, cmd := send(m, tea.MouseMotionMsg{X: from + 9, Y: bodyTop + line - m.s.top()})
	require.NotNil(t, cmd)
	send(m, hoverDue{Seq: m.s.Hover.Seq})

	require.Equal(t, 1, hv.HoverCallCount())
	assert.Equal(t, "b.go", hv.HoverArgsForCall(0).Path)
	assert.Equal(t, 1, hv.HoverArgsForCall(0).Line)
}

func TestTheCaretCrossesIntoTheNextFile(t *testing.T) {
	m, _ := streamModel(t)
	m, _ = typed(m, "i")
	require.True(t, m.s.Caret.On)
	require.Equal(t, 0, m.s.Caret.File)

	m, _ = typed(m, "j", "j", "j")

	assert.Equal(t, 1, m.s.Caret.File, "over b.go's header")
	m, _ = typed(m, "k", "k", "k")
	assert.Equal(t, 0, m.s.Caret.File, "and back")
}

func TestScrollingTheStreamAsksForTheNewWindow(t *testing.T) {
	m, _, _ := startModel(t, manyPaths(40)...)
	m, _ = typed(m, "s")
	sl := m.p.Stream.(*infakes.FakeStreamLoader)
	asked := sl.WantCallCount()

	m, _ = typed(m, "G")

	require.Greater(t, sl.WantCallCount(), asked)
	var paths []string
	for _, r := range sl.WantArgsForCall(sl.WantCallCount() - 1) {
		paths = append(paths, r.File.Path())
	}
	assert.Contains(t, paths, "f039.go", "the files at the end")
	m, _ = typed(m, "s")
	assert.False(t, m.s.Stream.On)
}

func manyPaths(n int) []string {
	fs := streamFiles(n)
	out := make([]string, n)
	for i, f := range fs {
		out[i] = f.Path()
	}
	return out
}

func TestTheStreamAsksNothingNewWhileNothingMoves(t *testing.T) {
	m, _ := streamModel(t)
	sl := m.p.Stream.(*infakes.FakeStreamLoader)
	asked := sl.WantCallCount()
	require.Equal(t, sl.WantArgsForCall(asked-1), m.s.Stream.Wanted, "the model keeps what it asked for")

	for x := range 10 {
		m, _ = send(m, tea.MouseMotionMsg{X: x, Y: bodyTop})
	}
	sl.DrainReturns([]filediff.Update{{Gen: m.s.Stream.Gen, File: m.s.Files[0], Err: assert.AnError}})
	m, _ = send(m, streamReady{})

	assert.Equal(t, asked, sl.WantCallCount())
}

func TestLeavingTheStreamEndsItsLoads(t *testing.T) {
	m, _, _ := startModel(t, manyPaths(40)...)
	m, _ = typed(m, "s")
	sl := m.p.Stream.(*infakes.FakeStreamLoader)
	require.NotEmpty(t, sl.WantArgsForCall(sl.WantCallCount()-1))

	m, _ = typed(m, "s")

	require.False(t, m.s.Stream.On)
	assert.Empty(t, sl.WantArgsForCall(sl.WantCallCount()-1))
}

func TestFilesBelowTheTopKeepTheMouseHovering(t *testing.T) {
	m, _ := streamModel(t)
	m.s.Stream.Held[0].servers = servers{}
	m.s.Stream.Held[0].snap.RightDoc = hover.Document{}
	require.False(t, m.s.fileHoverable(0))

	assert.True(t, m.s.hoverable(), "b.go below is served")
}
