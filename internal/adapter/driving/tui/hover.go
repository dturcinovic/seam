package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/view"
	"github.com/dturcinovic/seam/internal/textcell"
)

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
	// asked again once it is, or that came back empty, asked again once
	// the server's work ends.
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
	stream         view.StreamPos
	streamGen      filediff.Gen
	mode           view.Mode
	hScroll, lines int
	layout         layout
	cells          textcell.Method
	gen            filediff.Gen
	loaded         bool
}

func (s *state) screenKey() screenKey {
	return screenKey{s.Pos, s.Stream.Pos, s.Stream.Gen, s.Mode, s.HScroll, s.lineCount(), s.Layout, s.Cells, s.LoadGen, s.Loaded}
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
	if ok && h.On && t.File == h.Target.File && t.Side == h.Target.Side && t.Line == h.Target.Line && t.Word == h.Target.Word {
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
	p := s.positionAt(h.Target)
	h.Inflight, h.Waiting = p.Req, false
	s.LSP.Side = h.Target.Side
	return p, true
}

// positionAt is the position of t to ask the servers about, with a new
// ReqID; hovers and definitions share them.
func (s *state) positionAt(t hoverTarget) hover.Position {
	s.Hover.LastReq++
	d := s.snapOf(t.File).Doc(t.Side)
	return hover.Position{Req: s.Hover.LastReq, Commit: d.Commit, Path: d.Path, Version: d.Version, Line: t.Line, Byte: t.Byte}
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
// the popup, if it is about the text on screen; the server of the side
// asked about, or its checkout, becoming ready makes retry ask again, as
// does the end of its work for an answer that came empty.
func (s *state) hoverUpdate(u hover.Update) (retry bool) {
	h := &s.Hover
	switch u.Kind {
	case hover.StatusChanged, hover.CheckoutChanged, hover.ProgressChanged:
		if !h.Waiting {
			return false
		}
		retry, waiting := s.readyFor(u, h.Target)
		h.Waiting = waiting
		return retry
	case hover.HoverDone:
		if u.Req == 0 || u.Req != h.Inflight || u.Version != s.snapOf(h.Target.File).Doc(h.Target.Side).Version {
			return false
		}
		h.Inflight = 0
		switch u.Outcome {
		case hover.Text:
			h.Popup = s.openPopup(u.Content, h.X, h.Y)
		case hover.Empty:
			// A server knows little before its work, as indexing, ends; it
			// may answer before it even reports the work. Servers that
			// run one piece of work after another end each, so it is asked
			// again after every one.
			h.Waiting = true
		case hover.Error, hover.TimedOut:
		}
	case hover.DefinitionDone:
	}
	return false
}

// onMotion follows the mouse, and waits for it to rest on a new target:
// moving across the code asks nothing.
func (m *model) onMotion(x, y int) tea.Cmd {
	if l, ok := m.s.noteMarkAt(x, y); ok {
		if p := m.s.Hover.Popup; p.Notes == nil || *p.Notes != l {
			m.dropHover()
			m.s.openNotes(l, x, y)
		}
		return nil
	}
	cancel, due := m.s.hoverMove(x, y)
	m.cancel(cancel)
	if !due {
		return nil
	}
	seq := m.s.Hover.Seq
	return tea.Tick(m.hoverDelay, func(time.Time) tea.Msg { return hoverDue{Seq: seq} })
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
	if t, ok := m.s.caretTarget(); ok {
		cancel, seq := m.s.hoverNow(t, x, y)
		m.cancel(cancel)
		m.ask(seq)
	}
}

// caretTarget is what K and d ask about: the grapheme under the caret.
func (s *state) caretTarget() (hoverTarget, bool) {
	p, ok := s.spot()
	if !ok {
		return hoverTarget{}, false
	}
	c, w := s.Caret, s.caretSide(p.line)
	return targetOn(c.File, w, lineOf(s.snapOf(c.File).Diff.Rows[c.Row], w), p.text, p.c, hoverTarget{})
}

// readyFor says whether u makes a request for t, waiting for its server,
// worth asking again, and whether it is still to wait: a checkout that
// failed brings no server to wait for.
func (s *state) readyFor(u hover.Update, t hoverTarget) (retry, waiting bool) {
	switch u.Kind {
	case hover.StatusChanged:
		return u.Status == hover.Ready && u.Instance == s.instanceOf(t.File, t.Side), true
	case hover.CheckoutChanged:
		if u.Commit != s.snapOf(t.File).Doc(t.Side).Commit {
			return false, true
		}
		return u.Status == hover.Ready, u.Status != hover.ServerFailed
	case hover.ProgressChanged:
		return u.Progress == (hover.Progress{}) && u.Instance == s.instanceOf(t.File, t.Side), true
	case hover.HoverDone, hover.DefinitionDone:
	}
	return false, true
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
