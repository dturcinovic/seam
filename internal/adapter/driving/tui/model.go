package tui

import (
	tea "charm.land/bubbletea/v2"

	"seam/internal/app/port/in"
	"seam/internal/domain/change"
	"seam/internal/domain/filediff"
)

// Messages the bridge sends when a port has updates waiting; Update drains
// them all, so one message stands for any number of updates.
type (
	fileDiffsReady struct{}
	changesetReady struct{}
)

// model is the Bubble Tea model: the state, the ports Update calls and the
// theme View draws with.
type model struct {
	s      state
	files  in.FileDiffLoader
	reload in.ChangesetReloader
	theme  *theme
}

// newModel shows cs and asks for its first file at once, as Init cannot
// change the model; the answer arrives through the bridge.
func newModel(cs change.Changeset, files in.FileDiffLoader, reload in.ChangesetReloader) model {
	m := model{s: newState(cs), files: files, reload: reload, theme: newTheme()}
	m.load(m.s.open(0, false))
	return m
}

// Init starts nothing: loading began in newModel.
func (model) Init() tea.Cmd { return nil }

// load passes a request an action made on to the port.
func (m model) load(r filediff.Request, ok bool) {
	if ok {
		m.files.Load(r)
	}
}
