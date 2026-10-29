package tui

import (
	"errors"
	"slices"
	"strconv"

	"github.com/dturcinovic/seam/internal/domain/note"
	"github.com/dturcinovic/seam/internal/domain/ui"
)

// noteChoice is the choice of the agent session to send the notes to,
// which S opens over the body when more than one could take them. Sel is
// the session chosen, by its row.
type noteChoice struct {
	Open     bool
	Sessions []note.Session
	Sel      int
	box
}

// sending is a send asked for, Op, to the session of Client.
type sending struct {
	Op     note.Op
	Client string
}

// sendNotes sends the waiting notes to the one agent session there is, or
// asks to which.
func (m *model) sendNotes() {
	s := &m.s
	targets := s.Notes.Targets
	switch {
	case !s.Notes.waiting():
		s.Note = "no notes to send"
	case len(targets) == 0:
		s.Note = "no agent session in this worktree"
	case len(targets) == 1:
		m.sendTo(targets[0])
	default:
		m.dropHover()
		s.Notes.Choice = noteChoice{Open: true, Sessions: slices.Clone(targets)}
		s.layoutChoice()
	}
}

// waiting reports that a note of this branch waits for an agent, open or
// sent.
func (n *notesState) waiting() bool {
	return slices.ContainsFunc(n.Heads, func(h note.Head) bool { return h.State.Waits() && !h.Away })
}

func (m *model) sendTo(target note.Session) {
	m.s.Notes.Choice = noteChoice{}
	m.s.Notes.Sending = sending{Op: m.p.NoteSender.Send(target.ID), Client: target.Client}
}

// sentDone tells the outcome of the send asked for, if u is it. It says
// sent, never arrived: no agent tells whether it heard.
func (s *state) sentDone(u note.Update) bool {
	sd := s.Notes.Sending
	if sd.Op == 0 || u.Op != sd.Op {
		return false
	}
	s.Notes.Sending = sending{}
	switch {
	case errors.Is(u.Err, note.ErrNoSession):
		s.Note = "notes not sent: the agent session ended"
		return true
	case u.Err != nil:
		s.Note = "notes not sent: " + u.Err.Error()
		return true
	}
	s.Note = "notes sent to " + sd.Client + ", not taken yet"
	return true
}

// choiceKey takes a key while the choice is open: a session's number, or
// the file list's motions, enter and esc. It takes every key but
// quitting, so that no key changes the view under it.
func (m *model) choiceKey(key string) bool {
	c := &m.s.Notes.Choice
	if i, err := strconv.Atoi(key); err == nil && i >= 1 && i <= len(c.Sessions) {
		m.sendTo(c.Sessions[i-1])
		return true
	}
	k := m.s.Keys
	a, ok := k.Action(ui.ListLayer, key)
	if !ok {
		a, ok = k.Action(ui.ViewLayer, key)
		return !ok || a != ui.Quit
	}
	if mo, ok := motionOf(a); ok {
		c.Sel = min(max(c.Sel+mo.steps(len(c.Sessions), c.shown()), 0), len(c.Sessions)-1)
		m.s.layoutChoice()
		return true
	}
	switch a { //nolint:exhaustive // the choice's keys only
	case ui.Open:
		m.sendTo(c.Sessions[c.Sel])
	case ui.Back:
		*c = noteChoice{}
	}
	return true
}

// layoutChoice places the open choice in the middle of the body, its
// session chosen in view.
func (s *state) layoutChoice() {
	c := &s.Notes.Choice
	if !c.Open {
		return
	}
	c.Title = "send notes to"
	if s.centerBox(&c.box, min(s.boxMaxInner(), choiceCols)+popupFrame, len(c.Sessions)) {
		c.Scroll = min(max(c.Scroll, c.Sel-c.shown()+1), c.Sel)
	}
}

// choiceCols is the width of the choice's rows.
const choiceCols = 48

// choiceRows writes a session of the choice: its number, client and the
// time it started.
func (s *state) choiceRows(ln *line, j int) {
	c := &s.Notes.Choice
	ses := c.Sessions[j]
	lk := overviewLook(j == c.Sel)
	num := " "
	if j < 9 {
		num = strconv.Itoa(j + 1)
	}
	ln.spaces(itemPad+1, lk.text)
	ln.text(num, lk.mark)
	ln.spaces(1, lk.text)
	since := "since " + ses.Started.Local().Format(sessionTime)
	writeCut(ln, shownText(ses.Client), ln.free-itemPad-len(since), lk.text, false)
	ln.text(since, lk.dim)
	ln.spaces(ln.free, lk.text)
}
