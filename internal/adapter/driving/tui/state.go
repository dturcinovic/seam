package tui

import (
	"seam/internal/domain/change"
	"seam/internal/domain/filediff"
	"seam/internal/domain/view"
)

// contextRows is how many unchanged rows the hunk view keeps around a change.
const contextRows = 3

// chrome is the lines the view spends outside the panes: header and footer.
const chrome = 2

// state is what the TUI shows, as plain data: no ports, no channels. Only
// the actions change it.
type state struct {
	Comparison change.Comparison
	Files      []change.ChangedFile
	// Current indexes Files; -1 when there is none.
	Current int

	// LoadGen is the generation of the file asked for last; only its update
	// is shown. ReloadGen is the same for the changeset.
	LoadGen   filediff.Gen
	ReloadGen change.Gen

	// Snapshot is the current file once Loaded, and empty before.
	Snapshot filediff.Snapshot
	Loaded   bool
	// Err is the last failed load or reload, shown until the next success.
	Err error

	Mode    view.Mode
	Pos     view.Pos
	HScroll int
	Width   int
	Height  int
	// Lines is Snapshot laid out in Mode, kept in step by the actions.
	Lines []view.Line
}

// newState holds the files of cs in the hunk view, none of them open yet.
func newState(cs change.Changeset) state {
	return state{Comparison: cs.Comparison, Files: cs.Files, Current: -1, Mode: view.Hunks}
}

// file is the current file; ok is false when there is none.
func (s *state) file() (change.ChangedFile, bool) {
	if s.Current < 0 || s.Current >= len(s.Files) {
		return change.ChangedFile{}, false
	}
	return s.Files[s.Current], true
}

// bodyHeight is how many lines of the file fit on the screen.
func (s *state) bodyHeight() int {
	return max(s.Height-chrome, 1)
}
