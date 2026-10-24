package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"seam/internal/domain/filediff"
	"seam/internal/domain/hover"
	"seam/internal/domain/view"
	"seam/internal/textcell"
)

// hoverDelay is how long the mouse rests on a target before it is asked
// about: moving across the code asks nothing.
const hoverDelay = 300 * time.Millisecond

// hoverState is the hover under the mouse: the target it rests on, the
// request out for it and the popup with its answer.
type hoverState struct {
	// Target is what the mouse rests on, if On; X and Y are the cell, where
	// the popup opens.
	Target hoverTarget
	On     bool
	X, Y   int
	// Seq counts the targets, so a tick for one left since does nothing.
	Seq uint64
	// LastReq is the last ReqID handed out; Inflight the one waited for,
	// zero for none. Waiting is a request the server was not ready for,
	// asked again once it is.
	LastReq  hover.ReqID
	Inflight hover.ReqID
	Waiting  bool
	Popup    popup
}

// screenKey is what decides the text under each cell of the panes.
// A file asked for anew counts even with the same text: its other side, and
// so its rows, may differ.
type screenKey struct {
	pos            view.Pos
	mode           view.Mode
	hScroll, lines int
	layout         layout
	cells          textcell.Method
	gen            filediff.Gen
	loaded         bool
}

func (s *state) screenKey() screenKey {
	return screenKey{s.Pos, s.Mode, s.HScroll, len(s.Lines), s.Layout, s.Cells, s.LoadGen, s.Loaded}
}

// hoverDue is the tick that ends the mouse's rest on target Seq.
type hoverDue struct{ Seq uint64 }

// hoverMove follows the mouse to cell x, y. Over the popup, or within the
// identifier it rests on, nothing changes. Otherwise the popup closes and
// cancel names the request to drop; due says to wait for the mouse to rest
// on the new target.
func (s *state) hoverMove(x, y int) (cancel hover.ReqID, due bool) {
	h := &s.Hover
	if h.Popup.contains(x, y) {
		return 0, false
	}
	// A target left has an empty Word, which holds no grapheme.
	t, ok := s.targetAt(x, y, h.Target)
	if ok && h.On && t.Line == h.Target.Line && t.Word == h.Target.Word {
		// The popup opens where the mouse rests, not where it came in.
		if !h.Popup.Open {
			h.X, h.Y = x, y
		}
		return 0, false
	}
	if !ok {
		return s.hoverReset(), false
	}
	cancel, _ = s.hoverNow(t, x, y)
	return cancel, true
}

// hoverNow rests on t at once, at screen cell x, y, as K asks without
// waiting; seq is the tick to ask with and cancel the request to drop.
func (s *state) hoverNow(t hoverTarget, x, y int) (cancel hover.ReqID, seq uint64) {
	cancel = s.hoverReset()
	h := &s.Hover
	h.Target, h.On, h.X, h.Y = t, true, x, y
	h.Seq++
	return cancel, h.Seq
}

// hoverReset forgets the target and closes the popup, as when what is under
// the mouse moved; cancel names the request to drop.
func (s *state) hoverReset() (cancel hover.ReqID) {
	h := &s.Hover
	cancel = h.Inflight
	h.Target, h.On = hoverTarget{}, false
	h.Inflight, h.Waiting = 0, false
	h.Popup = popup{}
	return cancel
}

// hoverAsk is the position to ask about once tick seq is due, with a new
// ReqID that the answer must carry; ok is false for a target left since.
func (s *state) hoverAsk(seq uint64) (hover.Position, bool) {
	h := &s.Hover
	if !h.On || seq != h.Seq {
		return hover.Position{}, false
	}
	h.LastReq++
	h.Inflight, h.Waiting = h.LastReq, false
	d := s.Snapshot.RightDoc
	return hover.Position{Req: h.Inflight, Path: d.Path, Version: d.Version, Line: h.Target.Line, Byte: h.Target.Byte}, true
}

// hoverSubmitted takes how the request was taken. Only an answer to come
// keeps it in flight; a server not ready yet is asked again once it is.
func (s *state) hoverSubmitted(sub hover.Submit) {
	h := &s.Hover
	switch sub {
	case hover.Sent:
		return
	case hover.NotReady:
		h.Waiting = true
	case hover.Stale, hover.Busy:
		// Asked again on the next rest, even within the same identifier.
		h.Target, h.On = hoverTarget{}, false
	case hover.Unsupported, hover.Failed:
	}
	h.Inflight = 0
}

// hoverUpdate takes news from the servers: the answer waited for opens
// the popup, if it is about the text on screen; the server of the file
// shown becoming ready makes retry ask again.
func (s *state) hoverUpdate(u hover.Update) (retry bool) {
	h := &s.Hover
	switch u.Kind {
	case hover.StatusChanged:
		return h.Waiting && u.Status == hover.Ready && u.Instance == s.LSP.Shown
	case hover.HoverDone:
		if u.Req == 0 || u.Req != h.Inflight || u.Version != s.Snapshot.RightDoc.Version {
			return false
		}
		h.Inflight = 0
		if u.Outcome == hover.Text {
			h.Popup = s.openPopup(u.Content, h.X, h.Y)
		}
	}
	return false
}

// onMotion follows the mouse, and waits for it to rest on a new target.
func (m *model) onMotion(x, y int) tea.Cmd {
	cancel, due := m.s.hoverMove(x, y)
	m.cancel(cancel)
	if !due {
		return nil
	}
	seq := m.s.Hover.Seq
	return tea.Tick(hoverDelay, func(time.Time) tea.Msg { return hoverDue{Seq: seq} })
}

// hoverAtCaret asks about the grapheme under the caret, at once; the
// popup opens at its cell.
func (m *model) hoverAtCaret() {
	p, ok := m.s.spot()
	if !ok {
		return
	}
	x, y, ok := m.s.caretCell(p)
	if !ok {
		return
	}
	if t, ok := targetOn(m.s.Snapshot.Diff.Rows[m.s.Caret.Row].Right, p.text, p.c, hoverTarget{}); ok {
		cancel, seq := m.s.hoverNow(t, x, y)
		m.cancel(cancel)
		m.ask(seq)
	}
}

// ask asks about the target of tick seq, if the mouse still rests on it.
func (m *model) ask(seq uint64) {
	if p, ok := m.s.hoverAsk(seq); ok {
		m.s.hoverSubmitted(m.p.Hover.Hover(p))
	}
}

// dropHover closes the popup and forgets the target, as when what is under
// the mouse moves.
func (m *model) dropHover() { m.cancel(m.s.hoverReset()) }

func (m *model) cancel(id hover.ReqID) {
	if id != 0 {
		m.p.Hover.CancelHover(id)
	}
}
