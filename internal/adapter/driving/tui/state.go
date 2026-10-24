package tui

import (
	"seam/internal/domain/change"
	"seam/internal/domain/filediff"
	"seam/internal/domain/view"
	"seam/internal/textcell"
)

// contextRows is how many unchanged rows the hunk view keeps around a change.
const contextRows = 3

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

	List fileList
	// LSP is the language server of the file shown, for the footer.
	LSP lspState
	// Hover is what the mouse rests on and the popup with its answer.
	Hover hoverState
	// Caret is the keyboard's cursor in the right pane, for hovers.
	Caret caret

	Mode    view.Mode
	Pos     view.Pos
	HScroll int
	// Lines is Snapshot laid out in Mode, and Layout the screen for it at
	// the terminal's size; the actions keep both in step.
	Lines  []view.Line
	Layout layout
	// Marks is the scrollbar's tone for each line of the body, derived from
	// Lines at the body's height. It is kept rather than drawn per frame: a
	// file of many changes would cost milliseconds per frame.
	Marks []tone
	// Cells is how the terminal counts columns, which Bubble Tea's renderer
	// learns from it after the start.
	Cells textcell.Method
}

// newState holds the files of cs in the hunk view, none of them open yet.
func newState(cs change.Changeset) state {
	s := state{Comparison: cs.Comparison, Current: -1, Mode: view.Hunks}
	s.setFiles(cs.Files)
	s.place(0, 0)
	return s
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
	return s.Layout.Body
}
