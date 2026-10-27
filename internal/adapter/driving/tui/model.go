package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/ui"
)

// Messages the bridge sends when a port has updates waiting; Update drains
// them all, so one message stands for any number of updates.
type (
	fileDiffsReady struct{}
	streamReady    struct{}
	changesetReady struct{}
	lspUpdated     struct{}
	watchUpdated   struct{}
	notesUpdated   struct{}
)

// model is the Bubble Tea model: the state, the ports Update calls, the
// theme View draws with and how long the mouse rests before a hover.
type model struct {
	s          state
	p          Ports
	theme      *theme
	hoverDelay time.Duration
}

// newModel shows cs with the user's settings and asks for its first file
// at once, as Init cannot change the model; the answer arrives through the
// bridge.
func newModel(cs change.Changeset, p Ports, set ui.Settings) model {
	m := model{s: newState(cs, set), p: p, theme: newTheme(set.Palette), hoverDelay: set.HoverDelay}
	if !m.s.Stream.On {
		m.load(m.s.open(0, false))
	}
	m.want()
	return m
}

// Init starts nothing: loading began in newModel.
func (model) Init() tea.Cmd { return nil }

// want asks for the files the stream's window lacks, once the screen
// settled after a message.
func (m *model) want() {
	if !m.s.Stream.On {
		return
	}
	if rs, ok := m.s.streamWant(); ok {
		m.p.Stream.Want(rs)
	}
}

// load passes a request an action made on to the port.
func (m model) load(r filediff.Request, ok bool) {
	if ok {
		m.p.Files.Load(r)
	}
}
