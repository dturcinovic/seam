package tui

import (
	"cmp"
	"path"
	"slices"
	"strings"

	"seam/internal/domain/change"
	"seam/internal/domain/filediff"
	"seam/internal/textcell"
)

// The file list's width in columns, its separator not counted.
const (
	listMinWidth = 20
	listMaxWidth = 40
)

// minPaneWidth is how wide each pane must stay for the file list to open by
// itself.
const minPaneWidth = 40

// listChoice is whether the user opened or closed the file list. Until they
// do, the screen decides, see fileList.columns.
type listChoice uint8

const (
	listUnchosen listChoice = iota
	listOpened
	listClosed
)

// listItem is one line of the file list: the heading of a folder, with the
// number of its files, or a file, by its index.
type listItem struct {
	file  int
	dir   string
	count int
}

// heading reports whether the item is a folder's heading.
func (it listItem) heading() bool { return it.file < 0 }

// fileList is the sidebar's own state and rules, over files it does not
// hold. Cursor is a file and moves only while the list has the focus;
// Scroll is the first item shown. The items and their widths are built
// when the files or the way of counting cells change: a changeset may have
// tens of thousands of paths, and the layout asks on every change of file.
type fileList struct {
	Choice         listChoice
	Focus          bool
	Cursor, Scroll int

	// Items are the lines of the list, At the item of each file.
	Items []listItem
	At    []int
	// Longest is the width of the widest item, up to listMaxWidth, and
	// CountWidth that of each column of counts.
	Longest, CountWidth int
	Total               change.LineCounts
}

// sortByFolder orders files by folder and then by name, the order the list
// shows them in. By path alone, a/c/x.go would come between a/b.go and
// a/d.go and split a/ in two.
func sortByFolder(files []change.ChangedFile) []change.ChangedFile {
	type keyed struct {
		dir, name string
		file      change.ChangedFile
	}
	keys := make([]keyed, len(files))
	for i, f := range files {
		dir, name := path.Split(f.Path())
		keys[i] = keyed{dir, name, f}
	}
	slices.SortStableFunc(keys, func(a, b keyed) int {
		return cmp.Or(strings.Compare(a.dir, b.dir), strings.Compare(a.name, b.name))
	})
	sorted := make([]change.ChangedFile, len(keys))
	for i, k := range keys {
		sorted[i] = k.file
	}
	return sorted
}

// index builds the items of files, which sortByFolder ordered, and
// measures them.
func (l *fileList) index(files []change.ChangedFile, m textcell.Method) {
	l.Items, l.At = l.Items[:0], make([]int, len(files))
	l.Total = change.LineCounts{Known: true}
	heading := -1
	for i, f := range files {
		dir, _ := path.Split(f.Path())
		if heading < 0 || l.Items[heading].dir != dir {
			heading = len(l.Items)
			l.Items = append(l.Items, listItem{file: -1, dir: dir})
		}
		l.Items[heading].count++
		l.At[i] = len(l.Items)
		l.Items = append(l.Items, listItem{file: i})
		if f.Counts.Known {
			l.Total.Added += f.Counts.Added
			l.Total.Deleted += f.Counts.Deleted
		}
	}
	// No file counts more lines than all of them together.
	l.CountWidth = max(digits(max(l.Total.Added, l.Total.Deleted))+2, minCountWidth)
	l.Longest = totalsWidth(len(files), l.CountWidth, m)
	for _, it := range l.Items {
		if l.Longest >= listMaxWidth {
			break
		}
		l.Longest = max(l.Longest, itemWidth(it, files, l.CountWidth, m))
	}
	l.Longest = min(l.Longest, listMaxWidth)
}

// width is the list's width, its separator not counted, on a screen with
// room for it.
func (l *fileList) width() int {
	return max(l.Longest, listMinWidth)
}

// columns is how many columns the list and its separator take on a screen
// width columns wide: none while it is hidden, which it is as the user
// chose, or else unless there is more than one file and both panes keep
// minPaneWidth beside it.
func (l *fileList) columns(width int) int {
	w := min(l.width(), width/3)
	switch {
	case w <= 0 || l.Choice == listClosed:
		return 0
	case l.Choice == listUnchosen &&
		(len(l.At) < 2 || width-w-separatorWidth-scrollbarWidth < 2*minPaneWidth+separatorWidth):
		return 0
	}
	return w + separatorWidth
}

// move moves the cursor by d files and shows it in n lines.
func (l *fileList) move(d, n int) {
	l.Cursor = min(max(l.Cursor+d, 0), max(len(l.At)-1, 0))
	l.reveal(l.Cursor, n)
}

// jump moves the cursor to the first file of the next folder, or for a
// negative d to the first of its own; there already, to the first of the
// folder before.
func (l *fileList) jump(d, n int) {
	if len(l.At) == 0 {
		return
	}
	row := l.At[l.Cursor]
	target := l.heading(row-1, -1)
	switch {
	case d > 0:
		if target = l.heading(row+1, 1); target == len(l.Items) {
			return
		}
	case row == target+1 && target > 0:
		target = l.heading(target-1, -1)
	}
	l.Cursor = l.Items[target+1].file
	l.reveal(l.Cursor, n)
}

// heading is the first heading from item row on in direction step, or
// len(Items) past the last.
func (l *fileList) heading(row, step int) int {
	for row < len(l.Items) && !l.Items[row].heading() {
		row += step
	}
	return row
}

// page moves the cursor by d items, headings counted, so that a page of
// the list moves as far as the screen does; it lands on the file at or
// after a heading.
func (l *fileList) page(d, n int) {
	if len(l.At) == 0 {
		return
	}
	row := min(max(l.At[l.Cursor]+d, 1), len(l.Items)-1)
	if l.Items[row].heading() {
		row++
	}
	l.Cursor = l.Items[row].file
	l.reveal(l.Cursor, n)
}

// scroll moves the first item shown by d, keeping the last page full.
func (l *fileList) scroll(d, n int) {
	l.Scroll = min(max(l.Scroll+d, 0), max(len(l.Items)-n, 0))
}

// reveal scrolls as little as it takes to show file i, and the heading of
// its folder when it comes right before it.
func (l *fileList) reveal(i, n int) {
	if i < 0 || i >= len(l.At) {
		l.scroll(0, n)
		return
	}
	// Every file comes after a heading, so row is at least 1.
	row := l.At[i]
	top := row
	if l.Items[row-1].heading() {
		top = row - 1
	}
	l.Scroll = max(min(l.Scroll, top), row-max(n, 1)+1)
	l.scroll(0, n)
}

// follow keeps file i, now the one shown, in view, and takes the cursor
// along while the list has the focus.
func (l *fileList) follow(i, n int) {
	if l.Focus {
		l.Cursor = i
	}
	l.reveal(i, n)
}

// keep keeps in view what the list is about after its height changed: the
// cursor while it has the focus, else file current.
func (l *fileList) keep(current, n int) {
	if l.Focus {
		current = l.Cursor
	}
	l.reveal(max(current, 0), n)
}

// toggle is b: it opens a list that is not shown and gives it the focus,
// gives an open one the focus, and closes a focused one. Either way the
// user has chosen, and the screen no longer decides.
func (l *fileList) toggle(shown bool, current int) {
	if shown && l.Focus {
		l.Choice, l.Focus = listClosed, false
		return
	}
	l.Choice, l.Focus, l.Cursor = listOpened, true, max(current, 0)
}

func (l *fileList) blur() {
	l.Focus = false
}

// listLines is how many items the list on the screen shows.
func (s *state) listLines() int {
	return s.Layout.listEntries()
}

// toggleList is b, see fileList.toggle.
func (s *state) toggleList() {
	s.List.toggle(s.Layout.List > 0, s.Current)
	s.place(s.Layout.Width, s.Layout.Height)
	s.List.keep(s.Current, s.listLines())
}

func (s *state) moveCursor(n int) {
	s.List.move(n, s.listLines())
}

// pageList moves the cursor by d lines of the list, see fileList.page.
func (s *state) pageList(d int) {
	s.List.page(d, s.listLines())
}

// jumpFolder is [ and ] in the focused list, see fileList.jump.
func (s *state) jumpFolder(d int) {
	s.List.jump(d, s.listLines())
}

// scrollList moves the list's first item by n, as the wheel over it does,
// leaving the cursor where it is.
func (s *state) scrollList(n int) {
	s.List.scroll(n, s.listLines())
}

// openCursor is Enter in the list: it opens the file under the cursor,
// unless it is open already, and gives the focus back to the diff.
func (s *state) openCursor() (filediff.Request, bool) {
	s.List.blur()
	if s.Stream.On {
		s.streamJump(s.List.Cursor)
		return filediff.Request{}, false
	}
	if s.List.Cursor == s.Current {
		return filediff.Request{}, false
	}
	return s.open(s.List.Cursor, false)
}

// clickList opens the file at screen cell x, y of the list, as Enter does;
// a heading or the totals open nothing.
func (s *state) clickList(x, y int) (filediff.Request, bool) {
	i, ok := s.Layout.listLine(x, y)
	if !ok {
		return filediff.Request{}, false
	}
	j := s.List.Scroll + i
	if j >= len(s.List.Items) || s.List.Items[j].heading() {
		return filediff.Request{}, false
	}
	s.List.Cursor = s.List.Items[j].file
	return s.openCursor()
}
