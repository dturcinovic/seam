package tui

import (
	"slices"

	"seam/internal/domain/change"
	"seam/internal/domain/filediff"
	"seam/internal/domain/view"
)

// hStep is how far h and l move the panes sideways, in columns.
const hStep = 8

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
	}
	s.LoadGen++
	return filediff.Request{Gen: s.LoadGen, Comparison: s.Comparison, File: s.Files[i], Fresh: fresh}, true
}

// unload shows no file until the next one arrives.
func (s *state) unload() {
	s.Snapshot, s.Loaded, s.Lines = filediff.Snapshot{}, false, nil
	s.Pos, s.HScroll = view.Pos{}, 0
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
// left since is dropped.
func (s *state) apply(u filediff.Update) {
	if u.Gen != s.LoadGen {
		return
	}
	if u.Err != nil {
		s.Err = u.Err
		return
	}
	s.Snapshot, s.Loaded, s.Err = u.Snapshot, true, nil
	s.relayout()
}

// reload returns the generation of a new reload of the changeset.
func (s *state) reload() change.Gen {
	s.ReloadGen++
	return s.ReloadGen
}

// reloaded takes the new file list if u answers the last reload, and returns
// the fresh request for the file shown, or for the one that took its place
// if it is gone.
func (s *state) reloaded(u change.Update) (filediff.Request, bool) {
	if u.Gen != s.ReloadGen {
		return filediff.Request{}, false
	}
	if u.Err != nil {
		s.Err = u.Err
		return filediff.Request{}, false
	}
	shown, had := s.file()
	old := s.Current
	s.Files = u.Changeset.Files
	s.Current = slices.IndexFunc(s.Files, func(f change.ChangedFile) bool { return had && f.Path() == shown.Path() })
	if s.Current >= 0 {
		return s.open(s.Current, true)
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
	s.Width, s.Height = width, height
	s.Pos = view.Switch(s.Lines, s.Pos, s.bodyHeight())
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

// page moves by a screen, or by half of one.
func (s *state) page(dir int, half bool) {
	n := s.bodyHeight()
	if half {
		n = max(n/2, 1)
	}
	s.scrollBy(dir * n)
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

func (s *state) scrollTo(line int) {
	s.Pos = view.Navigate(s.Lines, s.Pos, line, s.bodyHeight())
}

// relayout lays out the diff again after it or the mode changed, keeping
// the anchor row.
func (s *state) relayout() {
	s.Lines = view.Lines(s.Snapshot.Diff, s.Mode, contextRows)
	s.Pos = view.Switch(s.Lines, s.Pos, s.bodyHeight())
}
