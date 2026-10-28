package tui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/dturcinovic/seam/internal/domain/note"
	"github.com/dturcinovic/seam/internal/domain/ui"
)

// deletionNameRunes is how much of a note's first line the question names
// it by.
const deletionNameRunes = 30

// noteDeletion is the question asked before a note leaves the book, which
// cannot be undone; question is its text for the footer. The zero value
// asks nothing.
type noteDeletion struct {
	id       note.ID
	question string
}

func (d noteDeletion) asking() bool { return d.id != "" }

// askDeletion asks whether to delete n, named by its kind and text, as a
// line's popup holds several and the overview's choice may move under the
// question. A note known only by its ID goes unnamed.
func (s *state) askDeletion(n note.Note) {
	name := "note"
	if n.Kind != 0 {
		name = n.Kind.String() + " “" + deletionName(n.Text) + "”"
	}
	s.Notes.Delete = noteDeletion{id: n.ID, question: "delete " + name + "? " + ui.ConfirmDeletion + " deletes, any other key keeps it"}
}

func deletionName(text string) string {
	first, _, cut := strings.Cut(text, "\n")
	if r := []rune(first); len(r) > deletionNameRunes {
		first, cut = string(r[:deletionNameRunes]), true
	}
	if cut {
		first += "…"
	}
	return shownText(first)
}

// deletePopupNote asks to delete the note x would resolve.
func (m *model) deletePopupNote() {
	if n, ok := m.s.popupNote(anyNote); ok {
		m.s.askDeletion(n)
	}
}

// deletionTakes lets the question take msg ahead of every layer, as the
// overview would swallow a click before it reached onClick. A key answers
// and a click keeps the note; the rest of the mouse rests, so that no
// popup of another note takes the place of the one asked about.
func (m *model) deletionTakes(msg tea.Msg) bool {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		m.answerDeletion(msg.String())
	case tea.MouseClickMsg:
		m.s.Notes.Delete = noteDeletion{}
		m.s.Note = "note kept"
	default:
		return mouseOrHover(msg)
	}
	return true
}

// answerDeletion deletes the note asked about if key confirms, and keeps
// it for any other. Either way the key does nothing else, so
// that esc keeps the popup the question was asked in.
func (m *model) answerDeletion(key string) {
	id := m.s.Notes.Delete.id
	m.s.Notes.Delete = noteDeletion{}
	if key != ui.ConfirmDeletion {
		m.s.Note = "note kept"
		return
	}
	if m.s.Hover.Popup.Note == id {
		m.dropHover()
	}
	m.p.NoteDeleter.Delete(id)
	m.s.Note = "note deleted"
}

// dropGoneDeletion ends the question once its note is no longer in the
// book. A book that cannot be read says nothing of the note: the question
// stays, and its answer then tells why it cannot delete.
func (s *state) dropGoneDeletion() {
	d := s.Notes.Delete
	if d.asking() && s.Notes.Err == nil && !slices.ContainsFunc(s.Notes.Heads, func(h note.Head) bool { return h.ID == d.id }) {
		s.Notes.Delete = noteDeletion{}
		s.Note = "note gone, nothing deleted"
	}
}
