package tui

import (
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/diff"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/view"
)

// jumpHistory bounds the places ctrl+o goes back to; the oldest falls out.
const jumpHistory = 50

// targetContext is how many rows stand above a definition jumped to, so
// that its comment shows too.
const targetContext = 3

// jumpState is go-to-definition: the request waited for, the places jumped
// from, and where the file jumped to is to stand once it has loaded.
type jumpState struct {
	// Inflight is the request waited for, zero for none; Waiting the target
	// whose server was not ready, asked again once it is.
	Inflight hover.ReqID
	Waiting  *hoverTarget
	// Back are the places jumped from, the last one last.
	Back []place
	// Target is the row to bring up once its file has loaded.
	Target *target
}

// place is where the viewer stood. A single file goes by its path, as a
// reload changes the indexes.
type place struct {
	Stream    bool
	StreamPos view.StreamPos
	Path      string
	Visit     bool
	Pos       view.Pos
	Mode      view.Mode
}

type target struct {
	File int
	Side filediff.Which
	Line int
}

// definitionSubmitted takes how the request for t was taken.
func (s *state) definitionSubmitted(t hoverTarget, req hover.ReqID, sub hover.Submit) {
	j := &s.Jump
	j.Inflight, j.Waiting = 0, nil
	switch sub {
	case hover.Sent:
		j.Inflight, s.Note = req, ""
	case hover.NotReady:
		j.Waiting = &t
		s.Note = "definition: language server starting…"
	case hover.Unsupported:
		s.Note = "definition: no language server here"
	case hover.Busy, hover.Stale:
		s.Note = "definition: language server busy, try again"
	case hover.Failed:
		s.Note = "definition: language server failed"
	}
}

// definitionRetry is the target waiting for its server if u makes it
// worth asking again.
func (s *state) definitionRetry(u hover.Update) (hoverTarget, bool) {
	w := s.Jump.Waiting
	if w == nil {
		return hoverTarget{}, false
	}
	retry, waiting := s.readyFor(u, *w)
	if !waiting {
		s.Jump.Waiting = nil
	}
	return *w, retry
}

// userActed forgets the definition waiting for its server and the row a
// jump is to bring up, once the user moves on: the answer, or the load,
// would come too late to be of use.
func (s *state) userActed(msg any) {
	switch msg.(type) {
	case tea.KeyPressMsg, tea.MouseClickMsg, tea.MouseWheelMsg:
		s.Jump.Waiting, s.Jump.Target = nil, nil
	}
}

// definitionDone takes the answer waited for: it jumps there, or says why
// not; r is the file to load for the jump.
func (s *state) definitionDone(u hover.Update) (filediff.Request, bool) {
	if u.Kind != hover.DefinitionDone || u.Req == 0 || u.Req != s.Jump.Inflight {
		return filediff.Request{}, false
	}
	s.Jump.Inflight = 0
	if u.Outcome != hover.Text {
		s.Note = missedText(u.Outcome)
		return filediff.Request{}, false
	}
	s.Note = ""
	r, ok := s.jumpTo(u.Target)
	if u.Targets > 1 && s.Note == "" {
		s.Note = fmt.Sprintf("definition 1 of %d", u.Targets)
	}
	return r, ok
}

// missedText is the footer's word on a definition not found.
func missedText(o hover.Outcome) string {
	switch o {
	case hover.Empty:
		return "no definition found"
	case hover.TimedOut:
		return "definition: no answer in time"
	case hover.Text, hover.Error:
	}
	return "definition: the server failed to answer"
}

// jumpTo shows l, remembering the place left. A file of the changeset
// shows in full, or in the stream where it stands; one of the worktree
// outside it as a file read only; anything else is named in the footer.
func (s *state) jumpTo(l hover.Location) (filediff.Request, bool) {
	if l.Outside {
		s.Note = fmt.Sprintf("definition in %s:%d", shownText(l.Path), l.Line+1)
		return filediff.Request{}, false
	}
	side := filediff.NewSide
	if left := s.Comparison.Left; !l.Commit.IsZero() && left.Kind() == change.KindCommit && l.Commit == left.ID() {
		side = filediff.OldSide
	}
	f := slices.IndexFunc(s.Files, func(c change.ChangedFile) bool {
		return side == filediff.OldSide && c.OldPath == l.Path || side == filediff.NewSide && s.serverPath(c) == l.Path
	})
	switch {
	case f >= 0:
		s.remember()
		s.Jump.Target = &target{File: f, Side: side, Line: l.Line}
		if s.Stream.On {
			s.streamJump(f)
			s.bringTarget()
			return filediff.Request{}, false
		}
		s.Mode = view.Full
		return s.showFile(f)
	case l.Commit.IsZero() && s.canVisit():
		s.remember()
		s.Jump.Target = &target{File: -1, Side: filediff.NewSide, Line: l.Line}
		if s.VisitPath == l.Path && s.Loaded {
			// Within the file visited: no need to read it again.
			s.bringTarget()
			return filediff.Request{}, false
		}
		s.Mode = view.Full
		return s.visit(l.Path)
	}
	s.Note = fmt.Sprintf("definition in %s:%d, not in this comparison", shownText(l.Path), l.Line+1)
	return filediff.Request{}, false
}

// showFile shows file f in the single view: at once if it is shown and
// loaded, else from a load, which the request asks for. The caller sets
// Pos or Target, which the load keeps.
func (s *state) showFile(f int) (filediff.Request, bool) {
	if f == s.Current && s.Loaded {
		s.relayout()
		s.bringTarget()
		return filediff.Request{}, false
	}
	return s.reopen(f, false)
}

// reopen opens f even if it is current, so that open places it anew.
func (s *state) reopen(f int, fresh bool) (filediff.Request, bool) {
	s.Current = -1
	return s.open(f, fresh)
}

// visit shows the worktree file at path, which is not in the changeset,
// read only.
func (s *state) visit(path string) (filediff.Request, bool) {
	if s.Stream.On {
		s.dropStream()
	}
	s.unload()
	s.Current, s.VisitPath = -1, path
	return s.visitRequest(false), true
}

// canVisit says whether a worktree file outside the changeset can be shown.
func (s *state) canVisit() bool {
	_, ok := s.Comparison.Visits()
	return ok
}

// visitRequest asks for the file visited, as a file with a new side only.
func (s *state) visitRequest(fresh bool) filediff.Request {
	s.LoadGen++
	file := change.ChangedFile{NewPath: s.VisitPath, NewMode: 0o100644}
	// Only a comparison that can visit has a file visited: jumpTo checks
	// canVisit, and a reload keeps the comparison's sources.
	c, _ := s.Comparison.Visits()
	return filediff.Request{Gen: s.LoadGen, Comparison: c, File: file, Fresh: fresh}
}

// serverPath is the path the language server knows the new side of c by:
// a file on disk is named as the user gave it, the server names its place
// in the worktree.
func (s *state) serverPath(c change.ChangedFile) string {
	if s.Comparison.Right.Kind() == change.KindPath {
		return c.WorktreePath
	}
	return c.NewPath
}

// visiting says the file shown is one outside the changeset, read only.
func (s *state) visiting() bool { return s.VisitPath != "" }

// bringTarget brings the row of the target up, once its file has its
// lines; a fold that hides it opens.
func (s *state) bringTarget() {
	t := s.Jump.Target
	if t == nil {
		return
	}
	snap, loaded := s.loadedOf(t.File)
	lines := s.linesOf(t.File)
	if !loaded || len(lines) == 0 {
		return
	}
	s.Jump.Target = nil
	row := slices.IndexFunc(snap.Diff.Rows, func(r diff.Row) bool { return lineOf(r, t.Side) >= t.Line })
	if row < 0 {
		row = len(snap.Diff.Rows) - 1
	}
	half := view.NewHalf
	if t.Side == filediff.OldSide {
		half = view.OldHalf
	}
	if i := view.LineForRow(lines, row); lines[i].Fold {
		s.unfoldLine(s.bodyIndex(t.File, i))
	}
	top := max(row-targetContext, 0)
	if s.Stream.On {
		s.Stream.Pos = view.StreamPos{File: t.File, Spot: view.OnRow, AnchorRow: top, AnchorHalf: half}
		s.switchStream()
		return
	}
	s.Pos = view.Switch(s.Lines, view.Pos{AnchorRow: top, AnchorHalf: half}, s.bodyHeight())
}

// remember keeps the place the viewer stands at for ctrl+o.
func (s *state) remember() {
	p := place{Mode: s.Mode, Stream: s.Stream.On, StreamPos: s.Stream.Pos, Visit: s.visiting(), Pos: s.Pos}
	if f, ok := s.file(); ok {
		p.Path = f.Path()
	}
	s.Jump.Back = append(s.Jump.Back, p)
	if len(s.Jump.Back) > jumpHistory {
		s.Jump.Back = slices.Delete(s.Jump.Back, 0, len(s.Jump.Back)-jumpHistory)
	}
}

// jumpBack returns to the place last jumped from, and returns the request
// for its file if it is to be loaded.
func (s *state) jumpBack() (filediff.Request, bool) {
	j := &s.Jump
	if len(j.Back) == 0 {
		return filediff.Request{}, false
	}
	p := j.Back[len(j.Back)-1]
	j.Back, j.Target = j.Back[:len(j.Back)-1], nil
	s.Mode = p.Mode
	if p.Stream {
		if !s.Stream.On {
			s.enterStream()
		}
		s.Stream.Pos = p.StreamPos
		s.layStream()
		return filediff.Request{}, false
	}
	if s.Stream.On {
		s.dropStream()
	}
	if p.Visit {
		r, ok := s.visit(p.Path)
		s.Pos = p.Pos
		return r, ok
	}
	f := slices.IndexFunc(s.Files, func(c change.ChangedFile) bool { return c.Path() == p.Path })
	if f < 0 {
		s.Note = "the file left is gone: " + shownText(p.Path)
		return filediff.Request{}, false
	}
	// Before for a file shown, which relayout keeps there; after for one
	// opened, as open starts it from the top.
	s.Pos = p.Pos
	r, ok := s.showFile(f)
	s.Pos = p.Pos
	return r, ok
}

// dropStream leaves the stream for a single file the caller opens. The
// single view's snapshot is from before the stream, so it goes too: kept,
// it would pass for the file now current.
func (s *state) dropStream() {
	s.Stream = streamState{Gen: s.Stream.Gen}
	s.Snapshot, s.Loaded, s.Lines = filediff.Snapshot{}, false, nil
}

// define asks where the symbol of t is defined, dropping the request
// waited for before.
func (m *model) define(t hoverTarget) {
	if id := m.s.Jump.Inflight; id != 0 {
		m.p.Define.CancelDefinition(id)
	}
	p := m.s.positionAt(t)
	m.s.definitionSubmitted(t, p.Req, m.p.Define.Define(p))
}

// defineAtCaret asks about the grapheme under the caret, as K hovers it.
func (m *model) defineAtCaret() {
	if t, ok := m.s.caretTarget(); ok {
		m.define(t)
	}
}

// onDefinition takes the servers' news for go-to-definition: the answer
// waited for, or a server ready for the target that waits for it.
func (m *model) onDefinition(u hover.Update) {
	if t, retry := m.s.definitionRetry(u); retry {
		m.define(t)
		return
	}
	m.load(m.s.definitionDone(u))
}
