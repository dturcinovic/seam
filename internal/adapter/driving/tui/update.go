package tui

import (
	tea "charm.land/bubbletea/v2"

	"seam/internal/textcell"
)

// Update applies a message. The port calls in it never block, so they run
// here directly rather than as commands, which Bubble Tea would not wait for
// on exit. Whatever moves the text under the mouse closes the popup, which
// was about the text that was there.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:ireturn // tea.Model prescribes it
	before := m.s.screenKey()
	m, cmd := m.update(msg)
	if m.s.screenKey() != before {
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
		for _, u := range m.p.LSP.Drain() {
			m.s.LSP.update(u)
			if m.s.hoverUpdate(u) {
				m.ask(m.s.Hover.Seq)
			}
		}
	case changesetReady:
		for _, u := range m.p.Reload.Drain() {
			before := m.s.Files
			m.load(m.s.reloaded(u))
			for _, path := range gone(before, m.s.Files) {
				m.p.Docs.Untrack(path)
			}
		}
	}
	return m, nil
}

// onKeyPress takes a key; Esc closes an open popup rather than quit.
func (m model) onKeyPress(key string) (model, tea.Cmd) {
	if m.s.Hover.Popup.Open && key == "esc" {
		m.dropHover()
		return m, nil
	}
	if m.s.List.Focus && m.onListKey(key) {
		return m, nil
	}
	return m.onKey(key)
}

// onKey maps a key, as Bubble Tea names it, to its action. Esc quits as long
// as there is no popup for it to close.
func (m model) onKey(key string) (model, tea.Cmd) {
	s := &m.s
	if mo, ok := motionOf(key); ok {
		s.scrollBy(mo.steps(len(s.Lines), s.bodyHeight()))
		return m, nil
	}
	switch key {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	case "r":
		m.p.Reload.Reload(s.reload())
	case "n":
		s.nextChange()
	case "p":
		s.prevChange()
	case "tab", "]":
		m.load(s.step(1))
	case "shift+tab", "[":
		m.load(s.step(-1))
	case "f":
		s.toggleMode()
	case "h", "left":
		s.shiftH(-1)
	case "l", "right":
		s.shiftH(1)
	case "b":
		s.toggleList()
	}
	return m, nil
}

// onListKey maps a key to its action in the focused file list; ok is false
// for the keys it leaves to onKey.
func (m *model) onListKey(key string) (ok bool) {
	s := &m.s
	if mo, ok := motionOf(key); ok {
		if mo.pages != 0 {
			s.pageList(mo.steps(len(s.Files), s.listLines()))
		} else {
			s.moveCursor(mo.steps(len(s.Files), s.listLines()))
		}
		return true
	}
	switch key {
	case "enter":
		m.load(s.openCursor())
	case "esc":
		s.List.blur()
	case "[":
		s.jumpFolder(-1)
	case "]":
		s.jumpFolder(1)
	default:
		return false
	}
	return true
}

// motion is where a navigation key moves, the diff or the focused file
// list alike: by lines, by pages or half pages, or to the start or end.
type motion struct {
	lines, pages, edge int
	half               bool
}

// motionOf is the motion of key; ok is false for any other key.
func motionOf(key string) (mo motion, ok bool) {
	switch key {
	case "j", "down":
		return motion{lines: 1}, true
	case "k", "up":
		return motion{lines: -1}, true
	case "ctrl+d":
		return motion{pages: 1, half: true}, true
	case "ctrl+u":
		return motion{pages: -1, half: true}, true
	case "pgdown", "space":
		return motion{pages: 1}, true
	case "pgup":
		return motion{pages: -1}, true
	case "g", "home":
		return motion{edge: -1}, true
	case "G", "end":
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
