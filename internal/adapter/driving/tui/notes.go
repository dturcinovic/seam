package tui

import (
	"fmt"
	"strings"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/diff"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/note"
)

// notesState is the review notes as the TUI knows them: the book in brief,
// where the notes of the sides on screen stand, the note being written and
// the line v marked.
type notesState struct {
	Heads []note.Head
	// Open counts the notes of Heads no agent answered yet, by file and
	// source; sent and taken ones too, as they still wait.
	Open map[headKey]int
	// Answered counts the notes answered and not yet resolved.
	Answered map[headKey]int
	// Sessions are the agent sessions of the book, to name who took a
	// note; Targets those of them whose process runs, to send notes to.
	Sessions []note.Session
	Targets  []note.Session
	// Err is why the book cannot be read or changed; Live that changes
	// from elsewhere show; Known that the book was read once.
	Err         error
	Live, Known bool
	// Spots are the notes of each side placed, by the side's text.
	Spots map[note.SnapshotKey][]note.Spot
	Draft noteDraft
	Mark  lineMark
	// Hidden hides the notes' lines below the code; Laid counts how often
	// they were laid in, which moves the lines of the body.
	Hidden bool
	Laid   int
	// Seek is the direction of m or M into a file just opened: to its
	// first or last note once they are placed; zero for none. At is the
	// line of the body m or M went to last, while the body is laid out as
	// AtLaid says.
	Seek   int
	At     int
	AtLaid laidKey
	// Discarded is an Add sent before its field was discarded, whose
	// outcome the footer still tells.
	Discarded note.Op
	// Asked are the sides whose placement was asked for and has not come.
	Asked map[note.SnapshotKey]bool
	// Overview is the list of the whole book; Find the note its enter
	// waits to show.
	Overview noteOverview
	Find     noteFind
	// Choice is the choice of the session to send to; Sending the send
	// whose outcome the footer tells.
	Choice  noteChoice
	Sending sending
	Delete  noteDeletion
}

// noteDraft is what the field writes: a note of Kind at Anchor. Op is the
// Add sent for it, zero while none waits for its outcome.
type noteDraft struct {
	Kind       note.Kind
	Anchor     note.Anchor
	Comparison string
	Op         note.Op
	// Lines are the lines noted as the screen shows them, where the field
	// stands.
	Lines noteTarget
}

// lineMark is a line marked with v, Row of Side of File: a note then takes
// the lines from it to the caret.
type lineMark struct {
	On   bool
	File int
	Side filediff.Which
	Row  int
}

// noteKey names side w of file as notes know it; ok is false for a side
// that takes none: one not loaded or without text, a submodule, a file on
// disk outside the worktree, and a worktree file whose blob is unknown.
func (s *state) noteKey(file int, w filediff.Which) (note.SnapshotKey, bool) {
	snap, loaded := s.loadedOf(file)
	if !loaded || len(sideOf(snap, w).Lines) == 0 {
		return note.SnapshotKey{}, false
	}
	f := snap.File
	src, path, blob, mode := s.Comparison.Left, f.OldPath, f.OldID, f.OldMode
	if w == filediff.NewSide {
		src, path, blob, mode = s.Comparison.Right, f.NewPath, f.NewID, f.NewMode
		switch {
		case s.visiting(), src.Kind() == change.KindWorktree:
			src, blob = change.Worktree(), snap.WorktreeBlob
		case src.Kind() == change.KindPath:
			src, path, blob = change.Worktree(), f.WorktreePath, snap.WorktreeBlob
		case src.Kind() == change.KindCommit, src.Kind() == change.KindIndex:
		}
	}
	origin, ok := note.OriginOf(src)
	if !ok || path == "" || blob.IsZero() || mode == change.Gitlink {
		return note.SnapshotKey{}, false
	}
	return note.SnapshotKey{Path: path, Origin: origin, Blob: blob}, true
}

// sideOf is side w of snap's diff.
func sideOf(snap *filediff.Snapshot, w filediff.Which) diff.Side {
	if w == filediff.OldSide {
		return snap.Diff.Old
	}
	return snap.Diff.New
}

// sideText is side w of snap as lines without their ends, as notes anchor
// them.
func sideText(snap *filediff.Snapshot, w filediff.Which) []string {
	lines := sideOf(snap, w).Lines
	text := make([]string, len(lines))
	for i, l := range lines {
		text[i] = diff.Content(l)
	}
	return text
}

// sideRef is where a side on screen is: side W of File.
type sideRef struct {
	File int
	W    filediff.Which
}

// noteSides are the sides on screen that take notes, by their key.
func (s *state) noteSides() map[note.SnapshotKey]sideRef {
	sides := make(map[note.SnapshotKey]sideRef)
	first, end := s.bodyFiles()
	if s.visiting() {
		first, end = s.Current, s.Current+1
	}
	for file := first; file < end; file++ {
		for _, w := range []filediff.Which{filediff.OldSide, filediff.NewSide} {
			if k, ok := s.noteKey(file, w); ok {
				sides[k] = sideRef{File: file, W: w}
			}
		}
	}
	return sides
}

// placeNotes asks where the notes of the sides on screen stand, and
// forgets the placements of sides no longer there. Unless again, it asks
// only for sides not placed yet: a side's key names its text, so their
// placement still holds.
func (m *model) placeNotes(again bool) {
	n := &m.s.Notes
	sides := m.s.noteSides()
	for k := range n.Spots {
		if _, ok := sides[k]; !ok {
			delete(n.Spots, k)
		}
	}
	for k := range n.Asked {
		if _, ok := sides[k]; !ok {
			delete(n.Asked, k)
		}
	}
	if n.Asked == nil {
		n.Asked = make(map[note.SnapshotKey]bool)
	}
	for k, ref := range sides {
		if _, placed := n.Spots[k]; (placed || n.Asked[k]) && !again {
			continue
		}
		n.Asked[k] = true
		m.p.NoteReader.Place(k, sideText(m.s.snapOf(ref.File), ref.W))
	}
}

// onNotes takes the news of the notes.
func (m *model) onNotes() {
	us := m.p.NoteUpdates.Drain()
	if len(us) == 0 {
		return
	}
	for _, u := range us {
		switch u.Kind {
		case note.Placed:
			m.s.placed(u)
		case note.Done:
			if !m.s.sentDone(u) {
				m.s.noteDone(u)
			}
		case note.StoreStatus:
			m.s.Notes.status(u)
			m.s.dropGoneDeletion()
			m.placeNotes(true)
			if m.s.Notes.Overview.Open {
				m.p.NoteOverview.Overview()
			}
		case note.Overview:
			m.s.takeOverview(u)
		}
	}
	m.s.refreshNotePopup()
}

// headKey is a file in a source, as the book counts its notes.
type headKey struct {
	Origin note.Origin
	Path   string
}

// status takes the book's state, and counts its open notes once rather
// than for every file in every frame.
func (n *notesState) status(u note.Update) {
	n.Heads, n.Sessions, n.Targets, n.Err, n.Live, n.Known = u.Heads, u.Sessions, u.Targets, u.Err, u.Live, true
	n.Open, n.Answered = make(map[headKey]int), make(map[headKey]int)
	for _, h := range u.Heads {
		switch h.State {
		case note.Open, note.Sent, note.Taken:
			n.Open[headKey{h.Origin, h.Path}]++
		case note.Answered:
			n.Answered[headKey{h.Origin, h.Path}]++
		case note.Resolved:
		}
	}
}

// placed keeps a placement of a side still on screen.
func (s *state) placed(u note.Update) {
	delete(s.Notes.Asked, u.Key)
	ref, ok := s.noteSides()[u.Key]
	if !ok {
		return
	}
	if s.Notes.Spots == nil {
		s.Notes.Spots = make(map[note.SnapshotKey][]note.Spot)
	}
	s.Notes.Spots[u.Key] = u.Spots
	s.annotateFile(ref.File)
	if ref.File == s.Current && !s.awaitsPlaces(ref.File) {
		s.seekPlaced()
	}
}

// awaitsPlaces reports that a side of file still waits for its
// placement.
func (s *state) awaitsPlaces(file int) bool {
	for _, w := range []filediff.Which{filediff.OldSide, filediff.NewSide} {
		if k, ok := s.noteKey(file, w); ok && s.Notes.Asked[k] {
			return true
		}
	}
	return false
}

// spotsOf are the notes placed in side w of file.
func (s *state) spotsOf(file int, w filediff.Which) []note.Spot {
	k, ok := s.noteKey(file, w)
	if !ok {
		return nil
	}
	return s.Notes.Spots[k]
}

// notesAt are the notes shown at line i of side w of file.
func (s *state) notesAt(file int, w filediff.Which, i int) []note.Spot {
	var at []note.Spot
	for _, sp := range s.spotsOf(file, w) {
		if sp.Place.Where.Shown() && sp.Place.Start-1 <= i && i < sp.Place.End {
			at = append(at, sp)
		}
	}
	return at
}

// noteMark is the gutter's mark of line i of side w of file in the role
// as: a diamond where the first note starts there, in its kind's colour,
// dimmed when only likely placed, or a dim bar within a note's lines.
func (s *state) noteMark(file int, w filediff.Which, i int) (mark string, as role) {
	for _, sp := range s.spotsOf(file, w) {
		switch p := sp.Place; {
		case !p.Where.Shown() || i < p.Start-1 || i >= p.End:
		case p.Start-1 == i:
			return noteStart, lookOf(sp.Note.Kind).marks(p.Where == note.Likely)
		default:
			mark, as = "╎", lookOf(sp.Note.Kind).dimMark
		}
	}
	return mark, as
}

// noteStart marks the first line of a note in the gutter.
const noteStart = "◆"

func kindMark(k note.Kind) string {
	if k == note.Finding {
		return "!"
	}
	return "?"
}

// noteSign is the sign of n in its line and in the overview: its kind's
// mark until it is answered.
func noteSign(n note.Note) string {
	if n.State == note.Answered || n.State == note.Resolved {
		return "✓"
	}
	return kindMark(n.Kind)
}

// notesText is the footer's word on the notes, empty while there are
// none: how many are open and answered in the comparison, how many of
// those on screen have no sure place, and whether changes from elsewhere
// show.
func (s *state) notesText() string {
	open, answered := 0, 0
	for i := range s.Files {
		o, a := s.notesIn(i)
		open, answered = open+o, answered+a
	}
	lost := 0
	for _, spots := range s.Notes.Spots {
		for _, sp := range spots {
			if !sp.Place.Where.Shown() {
				lost++
			}
		}
	}
	var parts []string
	if s.Notes.Err != nil {
		parts = append(parts, s.Notes.Err.Error())
	}
	if open > 0 {
		parts = append(parts, fmt.Sprintf("%d open", open))
	}
	if answered > 0 {
		parts = append(parts, fmt.Sprintf("%d answered", answered))
	}
	if lost > 0 {
		parts = append(parts, fmt.Sprintf("%d not placed", lost))
	}
	if s.Notes.Known && !s.Notes.Live {
		parts = append(parts, "not live")
	}
	if len(parts) == 0 {
		return ""
	}
	return "notes " + strings.Join(parts, ", ") + " · "
}
