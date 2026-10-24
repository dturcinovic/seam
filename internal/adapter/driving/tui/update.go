package tui

import (
	tea "charm.land/bubbletea/v2"

	"seam/internal/textcell"
)

// Update applies a message. The port calls in it never block, so they run
// here directly rather than as commands, which Bubble Tea would not wait for
// on exit.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:ireturn // tea.Model prescribes it
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.s.resize(msg.Width, msg.Height)
	case tea.ModeReportMsg:
		if reportsGraphemes(msg) {
			m.s.Cells = textcell.Graphemes
		}
	case tea.KeyPressMsg:
		return m.onKey(msg.String())
	case fileDiffsReady:
		for _, u := range m.files.Drain() {
			m.s.apply(u)
		}
	case changesetReady:
		for _, u := range m.reload.Drain() {
			m.load(m.s.reloaded(u))
		}
	}
	return m, nil
}

// onKey maps a key, as Bubble Tea names it, to its action. Esc quits as long
// as there is no popup for it to close.
func (m model) onKey(key string) (model, tea.Cmd) {
	s := &m.s
	switch key {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	case "r":
		m.reload.Reload(s.reload())
	case "j", "down":
		s.scrollBy(1)
	case "k", "up":
		s.scrollBy(-1)
	case "ctrl+d":
		s.page(1, true)
	case "ctrl+u":
		s.page(-1, true)
	case "pgdown", "space":
		s.page(1, false)
	case "pgup":
		s.page(-1, false)
	case "g", "home":
		s.scrollTo(0)
	case "G", "end":
		s.scrollTo(len(s.Lines))
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
	}
	return m, nil
}
