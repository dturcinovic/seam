package tui

import (
	"path"

	"seam/internal/domain/change"
	"seam/internal/domain/filediff"
	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
)

// lspState is what the footer tells of the language servers of the file
// shown. Statuses come per instance, whether its file is shown or not, so
// one shown again has its status at once.
type lspState struct {
	// servers serve the sides of the single file shown; zero for none,
	// which has no status. The stream keeps those of each file it holds.
	servers
	// Side is the side asked about last, whose server the footer names.
	Side   filediff.Which
	Status map[lang.Instance]hover.ServerStatus
	// Checkouts are how far the checkouts of commits have come, whose
	// documents have no server before.
	Checkouts map[change.ObjectID]hover.ServerStatus
}

// track tells the servers of the file just shown: both its sides, or that
// the worktree text a server may have had is no longer on screen, as when
// the file turned binary or too large.
func (m *model) track() {
	snap := &m.s.Snapshot
	m.s.LSP.Left, m.s.LSP.Right = m.tracked(snap.LeftDoc), m.tracked(snap.RightDoc)
	if path := snap.File.NewPath; snap.RightDoc.Path == "" && path != "" {
		m.p.Docs.Untrack(hover.DocKey{Path: path})
	}
}

// trackStream tells the servers of file of the stream, just loaded, as
// track does for the file shown.
func (m *model) trackStream(file int) {
	h := m.s.Stream.Held[file]
	h.servers = servers{m.tracked(h.snap.LeftDoc), m.tracked(h.snap.RightDoc)}
	if path := h.snap.File.NewPath; h.snap.RightDoc.Path == "" && path != "" {
		m.p.Docs.Untrack(hover.DocKey{Path: path})
	}
}

// tracked tells the servers of d and returns the instance serving it.
func (m *model) tracked(d hover.Document) lang.Instance {
	if d.Path == "" {
		return lang.Instance{}
	}
	inst, _ := m.p.Docs.Track(d)
	return inst
}

// retrack asks again for the instances of the sides of commit u, once its
// checkout is Ready: the documents waiting for it have servers now.
func (m *model) retrack(u hover.Update) {
	if u.Kind != hover.CheckoutChanged || u.Status != hover.Ready {
		return
	}
	if !m.s.Stream.On {
		m.retrackFile(&m.s.Snapshot, &m.s.LSP.servers, u.Commit)
		return
	}
	for _, h := range m.s.Stream.Held {
		m.retrackFile(&h.snap, &h.servers, u.Commit)
	}
}

// retrackFile asks for the servers of the sides of snap from commit.
func (m *model) retrackFile(snap *filediff.Snapshot, sv *servers, commit change.ObjectID) {
	if snap.LeftDoc.Commit == commit {
		sv.Left = m.tracked(snap.LeftDoc)
	}
	if snap.RightDoc.Commit == commit {
		sv.Right = m.tracked(snap.RightDoc)
	}
}

// gone are the paths of the right sides in before that after has not: a
// server must not keep their text, which a revert has made old.
func gone(before, after []change.ChangedFile) []string {
	kept := make(map[string]bool, len(after))
	for _, f := range after {
		kept[f.NewPath] = true
	}
	var out []string
	for _, f := range before {
		if f.NewPath != "" && !kept[f.NewPath] {
			out = append(out, f.NewPath)
		}
	}
	return out
}

// update takes a status change; answers are for the popup.
func (l *lspState) update(u hover.Update) {
	switch u.Kind {
	case hover.StatusChanged:
		if l.Status == nil {
			l.Status = make(map[lang.Instance]hover.ServerStatus)
		}
		l.Status[u.Instance] = u.Status
	case hover.CheckoutChanged:
		if l.Checkouts == nil {
			l.Checkouts = make(map[change.ObjectID]hover.ServerStatus)
		}
		l.Checkouts[u.Commit] = u.Status
	case hover.HoverDone, hover.DefinitionDone:
	}
}

// hoverable says that a server may answer hovers over a side of the file
// shown.
func (s *state) hoverable() bool {
	if !s.Stream.On {
		return s.fileHoverable(s.Current)
	}
	// Any file of the stream may be under the mouse, not only the top one.
	for f := range s.Stream.Held {
		if s.fileHoverable(f) {
			return true
		}
	}
	return false
}

func (s *state) fileHoverable(file int) bool {
	return s.sideHoverable(file, filediff.OldSide) || s.sideHoverable(file, filediff.NewSide)
}

// sideHoverable says that a server serves side w of file and is neither
// missing nor failed, or may once w's commit is checked out, which a hover
// starts.
func (s *state) sideHoverable(file int, w filediff.Which) bool {
	l := &s.LSP
	if inst := s.instanceOf(file, w); inst != (lang.Instance{}) {
		st := l.Status[inst]
		return st != hover.Missing && st != hover.ServerFailed
	}
	d := s.snapOf(file).Doc(w)
	st := l.Checkouts[d.Commit]
	return d.Path != "" && !d.Commit.IsZero() && st != hover.Ready && st != hover.ServerFailed
}

// named is the side whose server the footer names: the one asked about
// last, the right one before, or the one with a document at all.
func (s *state) named() filediff.Which {
	snap := s.snapOf(s.Current)
	if s.LSP.Side == filediff.OldSide && snap.LeftDoc.Path != "" || snap.RightDoc.Path == "" {
		return filediff.OldSide
	}
	return filediff.NewSide
}

// lspText is the footer's word on the server of a side of the file shown,
// as "gopls starting…", or "no LSP for .go" when it is not installed;
// empty when no server is meant to serve it. A commit's side says so, as
// "HEAD: gopls", and tells of its checkout until it has a server.
func lspText(s *state) string {
	l := &s.LSP
	w := s.named()
	inst, doc := s.instanceOf(s.Current, w), s.snapOf(s.Current).Doc(w)
	text := serverText(inst, l.Status[inst], doc)
	if inst == (lang.Instance{}) && !doc.Commit.IsZero() {
		text = checkoutText(l.Checkouts[doc.Commit])
	}
	if text == "" || doc.Commit.IsZero() {
		return text
	}
	src := s.Comparison.Right
	if w == filediff.OldSide {
		src = s.Comparison.Left
	}
	return shownText(src.String()) + ": " + text
}

func serverText(inst lang.Instance, st hover.ServerStatus, d hover.Document) string {
	name := shownText(inst.Server)
	switch st {
	case hover.Missing:
		return "no LSP for " + kindOf(d)
	case hover.Starting:
		return name + " starting…"
	case hover.Ready:
		return name
	case hover.ServerFailed:
		return name + " failed"
	case hover.Stopped:
		return name + " stopped"
	}
	return ""
}

// kindOf names the kind of file d is: its extension, else its language.
func kindOf(d hover.Document) string {
	if ext := path.Ext(d.Path); ext != "" {
		return shownText(ext)
	}
	return string(d.Lang)
}

// checkoutText is the footer's word on a checkout; once it is made, the
// server's word follows.
func checkoutText(st hover.ServerStatus) string {
	switch st {
	case hover.Starting:
		return "checkout…"
	case hover.ServerFailed:
		return "checkout failed"
	case hover.Missing, hover.Ready, hover.Stopped:
	}
	return ""
}
