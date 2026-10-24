package tui

import (
	tea "charm.land/bubbletea/v2"

	"seam/internal/domain/hover"
	"seam/internal/domain/ui"
	"seam/internal/textcell"
)

// Update applies a message. The port calls in it never block, so they run
// here directly rather than as commands, which Bubble Tea would not wait for
// on exit. Whatever moves the text under the mouse closes the popup, which
// was about the text that was there, and so does moving the caret, which
// in turn stays on the screen when the view or the text moves under it.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:ireturn // tea.Model prescribes it
	before, caret := m.s.screenKey(), m.s.Caret
	m, cmd := m.update(msg)
	m.s.keepCaret()
	if m.s.screenKey() != before || m.s.Caret != caret {
		m.dropHover()
	}
	return m, cmd
}

func (m model) update(msg tea.Msg) (model, tea.Cmd) {
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
		for _, u := range m.p.Files.Drain() {
			if m.s.apply(u) {
				m.track()
			}
		}
	case lspUpdated:
		m.onLSP()
	case changesetReady:
		for _, u := range m.p.Reload.Drain() {
			before := m.s.Files
			m.load(m.s.reloaded(u))
			for _, path := range gone(before, m.s.Files) {
				m.p.Docs.Untrack(hover.DocKey{Path: path})
			}
		}
	}
	return m, nil
}

// onLSP takes the servers' news.
func (m *model) onLSP() {
	for _, u := range m.p.LSP.Drain() {
		m.s.LSP.update(u)
		m.retrack(u)
		if m.s.hoverUpdate(u) {
			m.ask(m.s.Hover.Seq)
		}
	}
}

// onKeyPress takes a key, asking the layers from the top: an open popup,
// the focused file list, the caret, then the view.
func (m model) onKeyPress(key string) (model, tea.Cmd) {
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
	}
}

// onAction does an action of the view.
func (m model) onAction(a ui.Action) (model, tea.Cmd) {
	s := &m.s
	if mo, ok := motionOf(a); ok {
		s.scrollBy(mo.steps(len(s.Lines), s.bodyHeight()))
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
	case ui.ScrollLeft:
		s.shiftH(-1)
	case ui.ScrollRight:
		s.shiftH(1)
	case ui.ToggleFiles:
		s.toggleList()
	case ui.StartCaret:
		s.startCaret()
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
