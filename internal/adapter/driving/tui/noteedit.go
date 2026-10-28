package tui

import "github.com/dturcinovic/seam/internal/domain/note"

// noPlaceToEdit is the footer's word where a note stands at no line on
// screen: the field opens only between lines.
const noPlaceToEdit = "no place here to edit the note at"

// editPopupNote opens the field to rewrite the first note of the popup it
// may; where none may, the first says why. The popup of a single note is
// of one without a place, see enterNote.
func (m *model) editPopupNote() {
	at := m.s.Hover.Popup.Notes
	if at == nil {
		m.s.Note = noPlaceToEdit
		return
	}
	n, ok := m.s.popupNote(func(n note.Note) bool { return mayEdit(n.State) })
	if !ok {
		if first, some := m.s.popupNote(anyNote); some {
			m.s.editable(first)
		}
		return
	}
	ref := sideRef{File: at.File, W: at.Side}
	sp, ok := m.s.spotOf(n.ID, ref)
	if !ok {
		m.s.Note = "the notes are being placed again: E once they are"
		return
	}
	m.openEdit(ref, sp)
}

func mayEdit(st note.State) bool { return st != note.Resolved && st != note.Taken }

// editOverviewNote rewrites the note chosen in the overview, where enter
// would go to it.
func (m *model) editOverviewNote() {
	if n, ok := m.s.Notes.Overview.selected(); ok && m.s.editable(n) {
		m.load(m.s.enterNote(true))
	}
}

// editFound opens the field for the note E went to, once placed, as it
// stands now: another process or an agent may have changed it since E.
// No field opened meanwhile, as any key drops the find (userActed).
func (m *model) editFound(f found) {
	if m.s.editable(f.Spot.Note) {
		m.openEdit(f.Ref, f.Spot)
	}
}

// editable reports whether n may be rewritten, and the footer says why
// not.
func (s *state) editable(n note.Note) bool {
	switch n.State {
	case note.Resolved:
		s.Note = "a resolved note is not edited"
		return false
	case note.Taken:
		s.Note = "an agent holds this note: R gives it back"
		return false
	case note.Open, note.Sent, note.Answered:
	}
	return true
}

// openEdit opens the field at the lines of sp's note in side ref, with its
// kind and text and the cursor at the end.
func (m *model) openEdit(ref sideRef, sp note.Spot) {
	n := sp.Note
	// sp was placed in this side, so it has a key.
	k, _ := m.s.noteKey(ref.File, ref.W)
	m.s.Notes.Draft = noteDraft{
		Edit: n.ID, Kind: n.Kind, Anchor: n.Anchor, Blob: k.Blob,
		Lines: noteTarget{File: ref.File, Side: ref.W, From: sp.Place.Start - 1, To: sp.Place.End - 1},
	}
	m.openField(note.MaxText)
	m.s.fieldPaste(n.Text)
	if w := placeWord(sp.Place.Where); w != "" {
		// The field's opening cleared what targetSpot said.
		m.s.Note = w
	}
}
