package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/ui"
	"github.com/dturcinovic/seam/internal/textcell"
)

// Update applies a message. The port calls in it never block, so they run
// here directly rather than as commands, which Bubble Tea would not wait for
// on exit. Whatever moves the text under the mouse closes the popup, which
// was about the text that was there, and so does moving the caret, which
// in turn stays on the screen when the view or the text moves under it.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:ireturn // tea.Model prescribes it
	before, caret, stream := m.s.screenKey(), m.s.Caret, m.s.Stream.On
	m.s.userActed(msg)
	m, cmd := m.update(msg)
	if stream && !m.s.Stream.On {
		// However the stream was left, its loads would hold up the file
		// now asked for.
		m.p.Stream.Want(nil)
	}
	m.want()
	m.s.keepCaret()
	if m.s.screenKey() != before || m.s.Caret != caret {
		m.dropHover()
	}
	return m, cmd
}

func (m model) update(msg tea.Msg) (model, tea.Cmd) {
	if m.s.Field.Open && m.fieldTakes(msg) {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.s.resize(msg.Width, msg.Height)
	case tea.ModeReportMsg:
		if reportsGraphemes(msg) {
			m.s.useCells(textcell.Graphemes)
		}
	case tea.KeyPressMsg:
		return m.onKeyPress(msg.String())
	case tea.MouseWheelMsg:
		m.onWheel(msg)
	case tea.MouseClickMsg:
		m.onClick(msg)
	case tea.MouseMotionMsg:
		return m, m.onMotion(msg.X, msg.Y)
	case hoverDue:
		m.ask(msg.Seq)
	case fileDiffsReady:
		m.onFileDiffs()
	case streamReady:
		m.onStream()
	case lspUpdated:
		m.onLSP()
	case watchUpdated:
		m.onWatch()
	case changesetReady:
		m.onChangesets()
	}
	return m, nil
}

// fieldTakes gives the open field the keys and pastes, before any layer,
// and lets the mouse rest: there is no selecting in it, and the hovers
// would cover it. It reports whether msg is done with.
func (m *model) fieldTakes(msg tea.Msg) bool {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		m.s.fieldKey(msg.Key())
	case tea.PasteMsg:
		m.s.fieldPaste(msg.Content)
	case tea.MouseClickMsg, tea.MouseWheelMsg, tea.MouseMotionMsg, hoverDue:
	default:
		return false
	}
	return true
}

func (m *model) onFileDiffs() {
	for _, u := range m.p.Files.Drain() {
		if m.s.apply(u) {
			m.track()
		}
	}
}

func (m *model) onStream() {
	for _, f := range m.s.streamApply(m.p.Stream.Drain()) {
		m.trackStream(f)
	}
}

// onChangesets takes the reloads, and starts the one a change the watcher
// saw meanwhile waits for.
func (m *model) onChangesets() {
	for _, u := range m.p.Reload.Drain() {
		before := m.s.Files
		m.load(m.s.reloaded(u))
		for _, path := range gone(before, m.s.Files, m.s.serverPath) {
			m.p.Docs.Untrack(hover.DocKey{Path: path})
		}
	}
	if m.s.WatchAgain && !m.s.Reloading {
		m.s.WatchAgain = false
		m.watchReload()
	}
}

// onWatch takes the watcher's news. A change reloads as r does, which
// keeps the file and its place.
func (m *model) onWatch() {
	for _, u := range m.p.Watch.Drain() {
		switch u.Kind {
		case change.Watching:
			m.s.Watching = true
		case change.Changed:
			m.watchReload()
		case change.WatchFailed:
			m.s.Watching, m.s.WatchFailed, m.s.Err = false, true, u.Err
		}
	}
}

func (m *model) watchReload() {
	if gen, ok := m.s.watchReload(); ok {
		m.p.Reload.Reload(gen)
	}
}

// onLSP takes the servers' news.
func (m *model) onLSP() {
	for _, u := range m.p.LSP.Drain() {
		m.s.LSP.update(u)
		m.retrack(u)
		if m.s.hoverUpdate(u) {
			m.ask(m.s.Hover.Seq)
		}
		m.onDefinition(u)
	}
}

// onKeyPress takes a key, asking the layers from the top: an open popup,
// the focused file list, the caret, then the view.
func (m model) onKeyPress(key string) (model, tea.Cmd) {
	m.s.Note = ""
	k := m.s.Keys
	if _, ok := k.Action(ui.PopupLayer, key); ok && m.s.Hover.Popup.Open {
		m.dropHover()
		return m, nil
	}
	if a, ok := k.Action(ui.ListLayer, key); ok && m.s.List.Focus {
		m.onListAction(a)
		return m, nil
	}
	if a, ok := k.Action(ui.CaretLayer, key); ok && m.s.Caret.On {
		m.onCaretAction(a)
		return m, nil
	}
	if a, ok := k.Action(ui.ViewLayer, key); ok {
		return m.onAction(a)
	}
	return m, nil
}

// onCaretAction does an action of the caret.
func (m *model) onCaretAction(a ui.Action) {
	s := &m.s
	switch a { //nolint:exhaustive // the layer takes the caret's actions only
	case ui.LeaveCaret:
		s.Caret = caret{}
	case ui.CaretDown:
		s.caretUpDown(1)
	case ui.CaretUp:
		s.caretUpDown(-1)
	case ui.CaretLeft:
		s.caretSideways(-1)
	case ui.CaretRight:
		s.caretSideways(1)
	case ui.WordForward:
		s.wordForward()
	case ui.WordBack:
		s.wordBack()
	case ui.WordEnd:
		s.wordEnd()
	case ui.OtherSide:
		s.otherSide()
	case ui.Hover:
		m.hoverAtCaret()
	case ui.Definition:
		m.defineAtCaret()
	}
}

// onAction does an action of the view.
func (m model) onAction(a ui.Action) (model, tea.Cmd) {
	s := &m.s
	if mo, ok := motionOf(a); ok {
		s.scrollBy(mo.steps(s.lineCount(), s.bodyHeight()))
		return m, nil
	}
	switch a { //nolint:exhaustive // the layer takes the view's actions only
	case ui.Quit:
		return m, tea.Quit
	case ui.Reload:
		m.p.Reload.Reload(s.reload())
	case ui.NextChange:
		s.nextChange()
	case ui.PrevChange:
		s.prevChange()
	case ui.NextFile:
		m.load(s.step(1))
	case ui.PrevFile:
		m.load(s.step(-1))
	case ui.ToggleView:
		s.toggleMode()
	case ui.ToggleLayout:
		s.toggleLayout()
	case ui.ToggleStream:
		m.load(s.toggleStream())
	case ui.ScrollLeft:
		s.shiftH(-1)
	case ui.ScrollRight:
		s.shiftH(1)
	case ui.ToggleFiles:
		s.toggleList()
	case ui.StartCaret:
		s.startCaret()
	case ui.JumpBack:
		m.load(s.jumpBack())
	}
	return m, nil
}

// onListAction does an action of the focused file list.
func (m *model) onListAction(a ui.Action) {
	s := &m.s
	if mo, ok := motionOf(a); ok {
		if mo.pages != 0 {
			s.pageList(mo.steps(len(s.Files), s.listLines()))
		} else {
			s.moveCursor(mo.steps(len(s.Files), s.listLines()))
		}
		return
	}
	switch a { //nolint:exhaustive // the layer takes the list's actions only
	case ui.Open:
		m.load(s.openCursor())
	case ui.Back:
		s.List.blur()
	case ui.PrevFolder:
		s.jumpFolder(-1)
	case ui.NextFolder:
		s.jumpFolder(1)
	}
}

// motion is where a navigation key moves, the diff or the focused file
// list alike: by lines, by pages or half pages, or to the start or end.
type motion struct {
	lines, pages, edge int
	half               bool
}

// motionOf is the motion of a; ok is false for any other action.
func motionOf(a ui.Action) (mo motion, ok bool) {
	switch a { //nolint:exhaustive // the motions only
	case ui.Down:
		return motion{lines: 1}, true
	case ui.Up:
		return motion{lines: -1}, true
	case ui.HalfPageDown:
		return motion{pages: 1, half: true}, true
	case ui.HalfPageUp:
		return motion{pages: -1, half: true}, true
	case ui.PageDown:
		return motion{pages: 1}, true
	case ui.PageUp:
		return motion{pages: -1}, true
	case ui.Top:
		return motion{edge: -1}, true
	case ui.Bottom:
		return motion{edge: 1}, true
	}
	return motion{}, false
}

// steps is how many items mo moves among n with pages of height; moving to
// an edge moves by all of them, which the caller's bounds stop.
func (mo motion) steps(n, height int) int {
	switch {
	case mo.edge != 0:
		return mo.edge * n
	case mo.pages != 0:
		return mo.pages * pageLines(height, mo.half)
	}
	return mo.lines
}

// onClick opens the file clicked in the list; a click in the panes takes
// the focus from the list and unfolds a fold. Only the left button counts.
func (m *model) onClick(c tea.MouseClickMsg) {
	// A click on the popup is not for what lies under it.
	if c.Button != tea.MouseLeft || m.s.Hover.Popup.contains(c.X, c.Y) {
		return
	}
	m.s.Note = ""
	if c.Mod.Contains(tea.ModCtrl) {
		if t, ok := m.s.targetAt(c.X, c.Y, hoverTarget{}); ok {
			m.define(t)
		}
		return
	}
	if m.s.Layout.inList(c.X, c.Y) {
		m.load(m.s.clickList(c.X, c.Y))
		return
	}
	if _, ok := m.s.Layout.bodyLine(c.X, c.Y); ok {
		m.s.List.blur()
		m.s.unfoldAt(c.X, c.Y)
	}
}

// onWheel scrolls by a notch of the wheel: the file list when it is over
// it, else the panes. Shift turns it sideways, as do the wheel's own
// sideways buttons, which some terminals send for Shift or a touchpad.
func (m *model) onWheel(w tea.MouseWheelMsg) {
	if p := &m.s.Hover.Popup; p.contains(w.X, w.Y) && len(p.Lines) > p.shown() {
		if w.Button == tea.MouseWheelUp {
			p.scrollBy(-wheelLines)
		}
		if w.Button == tea.MouseWheelDown {
			p.scrollBy(wheelLines)
		}
		return
	}
	s, b, shift := &m.s, w.Button, w.Mod.Contains(tea.ModShift)
	if s.Layout.inList(w.X, w.Y) {
		// The list does not scroll sideways.
		if b == tea.MouseWheelUp {
			s.scrollList(-wheelLines)
		}
		if b == tea.MouseWheelDown {
			s.scrollList(wheelLines)
		}
		return
	}
	switch {
	case b == tea.MouseWheelLeft, b == tea.MouseWheelUp && shift:
		s.shiftH(-1)
	case b == tea.MouseWheelRight, b == tea.MouseWheelDown && shift:
		s.shiftH(1)
	case b == tea.MouseWheelUp:
		s.scrollBy(-wheelLines)
	case b == tea.MouseWheelDown:
		s.scrollBy(wheelLines)
	}
}
