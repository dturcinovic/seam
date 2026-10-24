package tui

import (
	"slices"

	"seam/internal/domain/change"
	"seam/internal/domain/filediff"
	"seam/internal/domain/lang"
	"seam/internal/domain/ui"
	"seam/internal/domain/view"
	"seam/internal/textcell"
)

// hStep is how far h and l, or a notch of the wheel sideways, move the
// panes, in columns.
const hStep = 8

// wheelLines is how far a notch of the wheel scrolls.
const wheelLines = 3

// The actions change the state and return what the ports should do; they do
// no I/O themselves, so they are tested without a terminal or ports.

// open makes file i current and returns the request for it; ok is false if
// i names no file. Another file starts at the top; the same file, read
// afresh, keeps its place and stays shown until the new one arrives.
func (s *state) open(i int, fresh bool) (r filediff.Request, ok bool) {
	if i < 0 || i >= len(s.Files) {
		return filediff.Request{}, false
	}
	if i != s.Current {
		s.unload()
		s.Current = i
		s.List.follow(i, s.listLines())
	}
	s.LoadGen++
	return filediff.Request{Gen: s.LoadGen, Comparison: s.Comparison, File: s.Files[i], Fresh: fresh}, true
}

// unload shows no file until the next one arrives.
func (s *state) unload() {
	s.Snapshot, s.Loaded = filediff.Snapshot{}, false
	s.LSP.Left, s.LSP.Right = lang.Instance{}, lang.Instance{}
	s.Pos, s.HScroll = view.Pos{}, 0
	s.Caret = caret{}
	s.place(s.Layout.Width, s.Layout.Height)
	s.show(nil)
}

// step opens the file n places away, staying at either end of the list.
func (s *state) step(n int) (filediff.Request, bool) {
	i := min(max(s.Current+n, 0), len(s.Files)-1)
	if i == s.Current {
		return filediff.Request{}, false
	}
	return s.open(i, false)
}

// apply shows u if it answers the last request; a late answer for a file
// left since is dropped. shown reports a new snapshot on screen.
func (s *state) apply(u filediff.Update) (shown bool) {
	if u.Gen != s.LoadGen {
		return false
	}
	if u.Err != nil {
		s.Err = u.Err
		return false
	}
	s.Snapshot, s.Loaded, s.Err = u.Snapshot, true, nil
	s.relayout()
	return true
}

// reload returns the generation of a new reload of the changeset.
func (s *state) reload() change.Gen {
	s.ReloadGen++
	s.Reloading = true
	return s.ReloadGen
}

// watchReload is reload for a change the watcher saw; ok is false while a
// reload runs, which a new one would cancel: on a repository that takes
// longer to list than a burst lasts, a busy writer would leave every reload
// unfinished. The change waits for the running reload's answer instead.
func (s *state) watchReload() (gen change.Gen, ok bool) {
	if s.Reloading {
		s.WatchAgain = true
		return 0, false
	}
	return s.reload(), true
}

// reloaded takes the new file list if u answers the last reload, and returns
// the fresh request for the file shown, or for the one that took its place
// if it is gone.
func (s *state) reloaded(u change.Update) (filediff.Request, bool) {
	if u.Gen != s.ReloadGen {
		return filediff.Request{}, false
	}
	s.Reloading = false
	if u.Err != nil {
		s.Err = u.Err
		return filediff.Request{}, false
	}
	shown, had := s.file()
	old := s.Current
	// A commit or a checkout may have moved the commits, and the files'
	// loads must ask for the new ones.
	s.Comparison = u.Changeset.Comparison
	s.setFiles(u.Changeset.Files)
	// open places the screen anew only for another file, and the list's
	// width and whether it shows depend on the files.
	s.place(s.Layout.Width, s.Layout.Height)
	s.List.Cursor = min(s.List.Cursor, max(len(s.Files)-1, 0))
	s.Current = slices.IndexFunc(s.Files, func(f change.ChangedFile) bool { return had && f.Path() == shown.Path() })
	if s.Current >= 0 {
		// The same file stays current, so open does not bring it into
		// view, though files before it may have come or gone.
		r, ok := s.open(s.Current, true)
		s.List.keep(s.Current, s.listLines())
		return r, ok
	}
	if len(s.Files) == 0 {
		s.unload()
		// A load still running is for a file that is gone.
		s.LoadGen++
		return filediff.Request{}, false
	}
	return s.open(min(max(old, 0), len(s.Files)-1), true)
}

// resize takes the new terminal size; the top line stays where it is.
func (s *state) resize(width, height int) {
	body := s.Layout.Body
	laid := s.place(width, height)
	s.List.keep(s.Current, s.listLines())
	if laid {
		return
	}
	if s.Layout.Body != body {
		s.mark()
	}
	s.Pos = view.Switch(s.Lines, s.Pos, s.bodyHeight())
}

// place lays out the screen for the current file, and its lines anew when
// the sides change between side by side and unified; laid says it did.
func (s *state) place(width, height int) (laid bool) {
	was := s.Layout.Unified
	s.Layout = computeLayout(width, height, s.List.columns(width), s.Snapshot.Diff, s.unified)
	if s.Layout.Unified != was {
		s.layLines()
		laid = true
	}
	// Only a list on the screen can have the focus, and a page that
	// shrank keeps its last line full.
	if s.Layout.List == 0 {
		s.List.blur()
	}
	s.scrollList(0)
	return laid
}

// setFiles is the one way to replace the files: in the order the list
// shows them, so that Tab and the list agree, and with the list built anew.
func (s *state) setFiles(files []change.ChangedFile) {
	s.Files = sortByFolder(files)
	s.List.index(s.Files, s.Cells)
}

// show is the one way to change Lines, so the scrollbar's marks follow.
func (s *state) show(lines []view.Line) {
	s.Lines = lines
	s.mark()
}

// mark derives the scrollbar's marks from Lines at the body's height.
func (s *state) mark() {
	s.Marks = changeMarks(s.Snapshot.Diff, s.Lines, s.Layout.Body)
}

// unified says whether a diff of width columns shows unified.
func (s *state) unified(width int) bool {
	switch s.LayoutSetting {
	case ui.SplitLayout:
		return false
	case ui.UnifiedLayout:
		return true
	case ui.AutoLayout:
	}
	return width < s.SplitMinWidth
}

// toggleLayout turns the sides the other way, for good: a resize after it
// leaves them so.
func (s *state) toggleLayout() {
	s.LayoutSetting = ui.UnifiedLayout
	if s.Layout.Unified {
		s.LayoutSetting = ui.SplitLayout
	}
	s.place(s.Layout.Width, s.Layout.Height)
}

// toggleMode switches between the hunk and the full view at the same row.
func (s *state) toggleMode() {
	if s.Mode == view.Full {
		s.Mode = view.Hunks
	} else {
		s.Mode = view.Full
	}
	s.relayout()
}

// scrollBy moves the top line by n lines.
func (s *state) scrollBy(n int) {
	s.scrollTo(s.Pos.Scroll + n)
}

// useCells takes the way the terminal counts columns, which the list's
// width depends on.
func (s *state) useCells(m textcell.Method) {
	s.Cells = m
	s.List.index(s.Files, m)
	s.place(s.Layout.Width, s.Layout.Height)
}

// pageLines is how far a page of height lines moves, or half a page.
func pageLines(height int, half bool) int {
	if half {
		return max(height/2, 1)
	}
	return height
}

// nextChange brings the first change below the top line to the top.
func (s *state) nextChange() {
	for _, c := range s.Snapshot.Diff.Changes {
		if line := view.LineForRow(s.Lines, c.Start); line > s.Pos.Scroll {
			s.scrollTo(line)
			return
		}
	}
}

// prevChange brings the last change above the top line to the top.
func (s *state) prevChange() {
	for _, c := range slices.Backward(s.Snapshot.Diff.Changes) {
		if line := view.LineForRow(s.Lines, c.Start); line < s.Pos.Scroll {
			s.scrollTo(line)
			return
		}
	}
}

// shiftH moves the panes sideways by n steps, never left of the start.
func (s *state) shiftH(n int) {
	s.HScroll = max(s.HScroll+n*hStep, 0)
}

// unfoldAt shows the rows of the fold at screen cell x, y one by one; a
// click anywhere else does nothing.
func (s *state) unfoldAt(x, y int) {
	i, ok := s.Layout.bodyLine(x, y)
	if !ok {
		return
	}
	lines, pos := view.Unfold(s.Lines, s.Pos, s.Pos.Scroll+i)
	// Unfold hands back the same lines when there was no fold, and then
	// the marks stand.
	if len(lines) != len(s.Lines) {
		s.Pos = pos
		s.show(lines)
	}
}

func (s *state) scrollTo(line int) {
	s.Pos = view.Navigate(s.Lines, s.Pos, line, s.bodyHeight())
}

// relayout lays out the diff again after it or the mode changed, keeping
// the anchor row.
func (s *state) relayout() {
	if !s.place(s.Layout.Width, s.Layout.Height) {
		s.layLines()
	}
}

// layLines lays out the diff in Mode, unified if Layout is, and keeps the
// anchor row on the screen.
func (s *state) layLines() {
	lines := view.Lines(s.Snapshot.Diff, s.Mode, s.Context)
	if s.Layout.Unified {
		lines = view.Unify(s.Snapshot.Diff, lines)
	}
	s.show(lines)
	s.Pos = view.Switch(s.Lines, s.Pos, s.bodyHeight())
}
