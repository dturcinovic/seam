package tui

import (
	"seam/internal/domain/change"
	"seam/internal/domain/filediff"
	"seam/internal/domain/ui"
	"seam/internal/domain/view"
	"seam/internal/textcell"
)

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
	// ShownGen is the generation of the snapshot shown: behind LoadGen
	// while an answer is still to come, as for a reload.
	ShownGen filediff.Gen

	// Snapshot is the current file once Loaded, and empty before.
	Snapshot filediff.Snapshot
	Loaded   bool
	// Err is the last failed load or reload, shown until the next success.
	Err error

	// Watching says that the watcher runs, and WatchFailed that it stopped;
	// the header shows both. Reloading is a reload asked for and not answered yet; WatchAgain a
	// change the watcher saw meanwhile, whose reload waits for that answer.
	Watching, WatchFailed, Reloading, WatchAgain bool

	List fileList
	// LSP is the language server of the file shown, for the footer.
	LSP lspState
	// Hover is what the mouse rests on and the popup with its answer.
	Hover hoverState
	// Caret is the keyboard's cursor in a pane, for hovers.
	Caret caret
	// Stream shows every file at once while On; Current, Snapshot, Loaded
	// and LSP then follow the file at its top, and Lines and Pos are unused.
	Stream streamState

	// Context is how many unchanged rows the hunk view keeps around a
	// change, and Keys what each key does; both are the user's settings.
	// KeyText is the footer's reminder of Keys, made once by setKeys, as
	// the footer is drawn with every frame.
	Context int
	Keys    ui.Keymap
	KeyText keyTexts
	// LayoutSetting is the setting ui.Layout, which with SplitMinWidth
	// decides whether Layout, the screen, is unified.
	LayoutSetting ui.Layout
	SplitMinWidth int

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
func newState(cs change.Changeset, set ui.Settings) state {
	s := state{
		Comparison: cs.Comparison, Current: -1, Mode: view.Hunks, Context: set.ContextLines,
		LayoutSetting: set.Layout, SplitMinWidth: set.SplitMinWidth,
	}
	s.setKeys(set.Keys)
	s.setFiles(cs.Files)
	s.place(0, 0)
	if set.View == ui.StreamView {
		s.enterStream()
	}
	return s
}

// setKeys takes k as the keys and their reminder.
func (s *state) setKeys(k ui.Keymap) {
	s.Keys, s.KeyText = k, newKeyTexts(k)
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
