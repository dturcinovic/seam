package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/app/port/in/infakes"
	"seam/internal/domain/change"
	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/ui"
	"seam/internal/textcell"
)

// manyFiles is n files f00.go, f01.go and so on.
func manyFiles(n int) []change.ChangedFile {
	var paths []string
	for i := range n {
		paths = append(paths, fmt.Sprintf("f%02d.go", i))
	}
	return files(paths...)
}

// listed is a state of 140 columns and 10 lines of body showing the first
// of n files, with the file list open by itself.
func listed(t *testing.T, n int) *state {
	t.Helper()
	d := diff.Compute(t.Context(), diff.Split(numbered(60)), diff.Split(numbered(60, 10, 40)))
	s := shown(t, manyFiles(n), 140, 10+chrome, filediff.Snapshot{Diff: d})
	require.Positive(t, s.Layout.List)
	return s
}

func TestTheListOpensByItselfWhenThereIsRoom(t *testing.T) {
	tests := []struct {
		name  string
		files int
		width int
		open  bool
	}{
		{"one file", 1, 200, false},
		{"two files, wide", 2, 140, true},
		{"two files, too narrow for two panes of 40", 2, 100, false},
		{"just wide enough", 2, 0, true},
		{"one column less", 2, -1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newState(change.Changeset{Files: manyFiles(tt.files)}, ui.DefaultSettings())
			if tt.width <= 0 {
				// The list's width plus two panes of minPaneWidth.
				list := s.List.width() + separatorWidth
				tt.width += list + 2*minPaneWidth + separatorWidth + scrollbarWidth
			}

			s.resize(tt.width, 20)

			assert.Equal(t, tt.open, s.Layout.List > 0)
		})
	}
}

func TestAnUnchosenListFollowsTheWindow(t *testing.T) {
	s := listed(t, 3)

	s.resize(100, 12)
	assert.Zero(t, s.Layout.List, "narrower")
	s.resize(140, 12)
	assert.Positive(t, s.Layout.List, "wide again")
}

func TestBOpensFocusesAndCloses(t *testing.T) {
	s := loaded(t)
	require.Zero(t, s.Layout.List, "80 columns are too narrow for it to open by itself")
	s.step(1)

	s.toggleList()
	assert.Positive(t, s.Layout.List, "b opens it")
	assert.True(t, s.List.Focus, "with the focus")
	assert.Equal(t, 1, s.List.Cursor, "on the file shown")

	s.List.blur()
	s.toggleList()
	assert.True(t, s.List.Focus, "b on an open list gives it the focus")
	assert.Positive(t, s.Layout.List)

	s.toggleList()
	assert.Zero(t, s.Layout.List, "b on a focused list closes it")
	assert.False(t, s.List.Focus)

	s.resize(200, 12)
	assert.Zero(t, s.Layout.List, "the user's choice stands on any screen")
}

func TestTheListTakesThePanesColumns(t *testing.T) {
	s := listed(t, 3)
	l := s.Layout

	assert.Equal(t, l.Width, l.List+l.panes()+l.scrollbar())
	assert.Equal(t, l.panes(), l.Left.Width+separatorWidth+l.Right.Width)
}

func TestTheCursor(t *testing.T) {
	s := listed(t, 30)
	s.toggleList()
	require.True(t, s.List.Focus)

	s.moveCursor(-1)
	assert.Zero(t, s.List.Cursor, "stays at the first file")
	s.moveCursor(12)
	assert.Equal(t, 12, s.List.Cursor)
	assert.Equal(t, 5, s.List.Scroll, "the list scrolls as little as it takes, its heading and totals counted")
	s.moveCursor(motion{pages: 1}.steps(len(s.Files), s.bodyHeight()))
	assert.Equal(t, 22, s.List.Cursor)
	s.moveCursor(motion{pages: -1, half: true}.steps(len(s.Files), s.bodyHeight()))
	assert.Equal(t, 17, s.List.Cursor)
	s.moveCursor(100)
	assert.Equal(t, 29, s.List.Cursor, "stays at the last")
	assert.Equal(t, 22, s.List.Scroll)
	assert.Zero(t, s.Current, "moving the cursor opens nothing")
}

func TestEnterOpensTheFileUnderTheCursor(t *testing.T) {
	s := listed(t, 5)
	s.toggleList()
	s.moveCursor(3)

	r, ok := s.openCursor()

	require.True(t, ok)
	assert.Equal(t, "f03.go", r.File.Path())
	assert.False(t, s.List.Focus, "the focus goes back to the diff")
	s.toggleList()
	_, ok = s.openCursor()
	assert.False(t, ok, "the file shown is not asked for again")
}

func TestTheListKeepsTheFileShownInView(t *testing.T) {
	s := listed(t, 30)

	for range 15 {
		s.step(1)
	}

	assert.Equal(t, 15, s.Current)
	assert.Equal(t, 8, s.List.Scroll)
}

func TestClickingAnEntryOpensIt(t *testing.T) {
	s := listed(t, 30)
	s.scrollList(5)

	r, ok := s.clickList(3, bodyTop+2)

	require.True(t, ok)
	assert.Equal(t, "f06.go", r.File.Path(), "below the heading of ./")
	assert.Equal(t, 6, s.List.Cursor)
}

func TestAClickBesideTheEntriesOpensNothing(t *testing.T) {
	few := listed(t, 3)
	tests := []struct {
		name string
		x, y int
	}{
		{"a heading", 3, bodyTop},
		{"the separator", few.Layout.List - separatorWidth, bodyTop + 1},
		{"below the last file", 3, bodyTop + 4},
		{"the header", 3, 0},
		{"the footer", 3, bodyTop + few.Layout.Body},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := listed(t, 3)

			_, ok := s.clickList(tt.x, tt.y)

			assert.False(t, ok)
		})
	}
}

func TestScrollingTheListStaysWithinIt(t *testing.T) {
	s := listed(t, 30)

	s.scrollList(-3)
	assert.Zero(t, s.List.Scroll)
	s.scrollList(100)
	assert.Equal(t, 22, s.List.Scroll, "the last page stays full")

	few := listed(t, 3)
	few.scrollList(3)
	assert.Zero(t, few.List.Scroll, "a list that fits does not scroll")
}

func TestAReloadReplacesTheList(t *testing.T) {
	s := listed(t, 30)
	s.toggleList()
	s.moveCursor(29)
	gen := s.reload()

	s.reloaded(change.Update{Gen: gen, Changeset: change.Changeset{Files: manyFiles(4)}})

	assert.Equal(t, 3, s.List.Cursor, "the cursor stays on a file")
	assert.LessOrEqual(t, s.List.Scroll, 3)
}

func TestTheListGroupsByFolder(t *testing.T) {
	s := newState(change.Changeset{Files: files("a/d.go", "a/c/x.go", "b.go", "a/b.go")}, ui.DefaultSettings())

	var got []string
	for _, it := range s.List.Items {
		if it.heading() {
			got = append(got, fmt.Sprintf("%s(%d)", folderText(it.dir), it.count))
		} else {
			got = append(got, s.Files[it.file].Path())
		}
	}

	assert.Equal(t, []string{"./(1)", "b.go", "a/(2)", "a/b.go", "a/d.go", "a/c/(1)", "a/c/x.go"}, got,
		"a folder's files stay together, and Tab follows the list")
	for i, row := range s.List.At {
		assert.Equal(t, i, s.List.Items[row].file)
	}
}

func TestBracketsJumpBetweenFolders(t *testing.T) {
	s := newState(change.Changeset{Files: files("a/1.go", "a/2.go", "b/1.go", "c/1.go", "c/2.go")}, ui.DefaultSettings())
	s.resize(140, 20)
	s.toggleList()
	s.moveCursor(1)
	path := func() string { return s.Files[s.List.Cursor].Path() }
	require.Equal(t, "a/2.go", path())

	s.jumpFolder(1)
	assert.Equal(t, "b/1.go", path())
	s.jumpFolder(1)
	assert.Equal(t, "c/1.go", path())
	s.jumpFolder(1)
	assert.Equal(t, "c/1.go", path(), "no folder after the last")
	s.moveCursor(1)
	s.jumpFolder(-1)
	assert.Equal(t, "c/1.go", path(), "back to the start of its own folder")
	s.jumpFolder(-1)
	assert.Equal(t, "b/1.go", path(), "from there to the folder before")
	s.jumpFolder(-1)
	s.jumpFolder(-1)
	assert.Equal(t, "a/1.go", path())
}

func TestTheFocusedListTakesItsKeys(t *testing.T) {
	fl, rl := &infakes.FakeFileDiffLoader{}, &infakes.FakeChangesetReloader{}
	m := model{s: *listed(t, 5), p: Ports{Files: fl, Reload: rl}}
	press := func(k tea.KeyPressMsg) (cmd tea.Cmd) {
		m, cmd = key(m, k)
		return cmd
	}

	press(tea.KeyPressMsg{Code: 'b', Text: "b"})
	require.True(t, m.s.List.Focus)
	press(tea.KeyPressMsg{Code: 'j', Text: "j"})
	press(tea.KeyPressMsg{Code: 'j', Text: "j"})
	assert.Equal(t, 2, m.s.List.Cursor, "j moves the cursor")
	assert.Zero(t, m.s.Pos.Scroll, "not the diff")
	press(tea.KeyPressMsg{Code: 'G', Text: "G"})
	assert.Equal(t, 4, m.s.List.Cursor)
	press(tea.KeyPressMsg{Code: 'g', Text: "g"})
	assert.Zero(t, m.s.List.Cursor)
	press(tea.KeyPressMsg{Code: 'n', Text: "n"})
	assert.Positive(t, m.s.Pos.Scroll, "the other keys still work the diff")

	assert.Nil(t, press(tea.KeyPressMsg{Code: tea.KeyEscape}), "esc does not quit")
	assert.False(t, m.s.List.Focus, "it gives the focus back")
	press(tea.KeyPressMsg{Code: tea.KeyEscape})
	press(tea.KeyPressMsg{Code: 'b', Text: "b"})
	press(tea.KeyPressMsg{Code: tea.KeyDown})
	press(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Equal(t, 1, fl.LoadCallCount())
	assert.Equal(t, "f01.go", fl.LoadArgsForCall(0).File.Path())
}

func TestTheMouseOverTheList(t *testing.T) {
	fl := &infakes.FakeFileDiffLoader{}
	m := model{s: *listed(t, 30), p: Ports{Files: fl}}
	mouse := func(msg tea.Msg) { m, _ = send(m, msg) }

	mouse(tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 2, Y: bodyTop})
	assert.Equal(t, wheelLines, m.s.List.Scroll, "the wheel over the list scrolls the list")
	assert.Zero(t, m.s.Pos.Scroll)
	mouse(tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: m.s.Layout.List + 5, Y: bodyTop})
	assert.Equal(t, wheelLines, m.s.Pos.Scroll, "over the panes the panes")

	mouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: 2, Y: bodyTop})
	require.Equal(t, 1, fl.LoadCallCount())
	assert.Equal(t, "f02.go", fl.LoadArgsForCall(0).File.Path(), "the first line the scrolled list shows")

	m.s.toggleList()
	require.True(t, m.s.List.Focus)
	mouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: m.s.Layout.List + 5, Y: bodyTop + 1})
	assert.False(t, m.s.List.Focus, "a click in the panes takes the focus")
}

func TestAFoldBesideTheListUnfolds(t *testing.T) {
	s := listed(t, 3)
	require.True(t, s.Lines[0].Fold)
	folded := len(s.Lines)

	s.unfoldAt(s.Layout.List-1, bodyTop)
	assert.Len(t, s.Lines, folded, "the separator is no part of the panes")
	s.unfoldAt(s.Layout.List, bodyTop)
	assert.Greater(t, len(s.Lines), folded)
}

func TestTheListIsMeasuredAgainWhenTheCellsAreCountedOtherwise(t *testing.T) {
	// A family emoji is one glyph to a terminal in grapheme mode and three
	// wide ones otherwise.
	family := strings.Repeat("👨\u200d👩\u200d👧", 3) + ".txt"
	s := newState(change.Changeset{Files: files(family, "b.go")}, ui.DefaultSettings())
	s.resize(200, 12)
	before := s.List.Longest

	s.useCells(textcell.Graphemes)

	assert.Less(t, s.List.Longest, before)
	assert.Equal(t, s.List.width()+separatorWidth, s.Layout.List)
}

func TestBOnAListThatOpenedByItselfIsAChoice(t *testing.T) {
	s := listed(t, 3)

	s.toggleList()
	s.resize(100, 12)

	assert.Positive(t, s.Layout.List, "it stays when the window narrows")
	assert.True(t, s.List.Focus)
}

func TestOnlyAListOnTheScreenHasTheFocus(t *testing.T) {
	t.Run("closed by the window", func(t *testing.T) {
		s := listed(t, 3)
		s.List.Focus = true

		s.resize(100, 12)

		require.Zero(t, s.Layout.List)
		assert.False(t, s.List.Focus)
	})
	t.Run("too narrow to open", func(t *testing.T) {
		s := listed(t, 3)
		s.resize(2, 12)

		s.toggleList()

		assert.Zero(t, s.Layout.List)
		assert.False(t, s.List.Focus, "j and k still scroll the diff")
	})
}

func TestAResizeKeepsTheListInView(t *testing.T) {
	s := listed(t, 30)
	for range 29 {
		s.step(1)
	}
	require.Equal(t, 22, s.List.Scroll)

	s.resize(140, 25+chrome)
	assert.Equal(t, 7, s.List.Scroll, "a taller list keeps its last page full")
	s.resize(140, 5+chrome)
	assert.Equal(t, 27, s.List.Scroll, "a shorter one keeps the file shown in view")
}

func TestTabTakesTheCursorAlong(t *testing.T) {
	s := listed(t, 30)
	s.toggleList()

	for range 15 {
		s.step(1)
	}

	assert.Equal(t, 15, s.List.Cursor)
	assert.Equal(t, 8, s.List.Scroll, "the file shown is in view")
}

func TestTheWheelOverTheHeaderScrollsTheDiff(t *testing.T) {
	m := model{s: *listed(t, 30)}

	m, _ = send(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 2, Y: 0})

	assert.Zero(t, m.s.List.Scroll)
	assert.Equal(t, wheelLines, m.s.Pos.Scroll)
}

func TestAReloadKeepsTheFileShownInView(t *testing.T) {
	s := listed(t, 30)
	for range 25 {
		s.step(1)
	}
	require.Positive(t, s.List.Scroll)
	gen := s.reload()

	s.reloaded(change.Update{Gen: gen, Changeset: change.Changeset{Files: manyFiles(30)}})

	row := s.List.At[s.Current]
	assert.True(t, s.List.Scroll <= row && row < s.List.Scroll+s.listLines(), "f25 is on screen")
}

func TestAPageSkipsNoLine(t *testing.T) {
	var fs []change.ChangedFile
	for _, dir := range []string{"a", "b", "c"} {
		for i := range 6 {
			fs = append(fs, files(fmt.Sprintf("%s/%d.go", dir, i))...)
		}
	}
	st := newState(change.Changeset{Files: fs}, ui.DefaultSettings())
	st.resize(140, 6+chrome)
	st.toggleList()
	m := model{s: st}
	n := m.s.listLines()

	for range 4 {
		before := m.s.List.Scroll
		m, _ = key(m, tea.KeyPressMsg{Code: tea.KeyPgDown})
		assert.LessOrEqual(t, m.s.List.Scroll, before+n, "the new page starts no later than right after the old one")
	}
	assert.Equal(t, len(fs)-1, m.s.List.Cursor)
	for range 6 {
		m, _ = key(m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	}
	assert.Zero(t, m.s.List.Cursor, "back at the first file, past its heading")
}

func TestAClickOutsideThePanesKeepsTheFocus(t *testing.T) {
	m := model{s: *listed(t, 3)}
	m.s.toggleList()
	require.True(t, m.s.List.Focus)

	m, _ = send(m, tea.MouseClickMsg{Button: tea.MouseLeft, X: 60, Y: 0})
	m, _ = send(m, tea.MouseClickMsg{Button: tea.MouseLeft, X: m.s.Layout.Width - 1, Y: bodyTop})

	assert.True(t, m.s.List.Focus, "neither the header nor the scrollbar takes it")
}
