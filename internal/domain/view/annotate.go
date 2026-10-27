package view

import "slices"

// NoteAt asks for a note line below the line that shows Half of Row; Ref is
// the caller's, handed back by Line.Note.
type NoteAt struct {
	Row  int
	Half Half
	Ref  int
}

// Annotate is lines with a note line below the line each of notes names,
// in the order given, and without the note lines lines had. A note in a
// fold stands below the fold, so that the hunk view shows it too. It runs
// last, on lines laid out by Lines, Unify and Unfold: those take no note
// lines.
//
// A note line repeats the rows and half of the line it follows. The search
// for a row then finds that line first, never the note line, and the run
// of halves of a unified block goes on past it.
func Annotate(lines []Line, notes []NoteAt) []Line {
	plain := lines
	if slices.ContainsFunc(lines, isNote) {
		plain = slices.DeleteFunc(slices.Clone(lines), isNote)
	}
	if len(notes) == 0 || len(plain) == 0 {
		return plain
	}
	below := make(map[int][]int, len(notes))
	for _, n := range notes {
		i := LineFor(plain, n.Row, n.Half)
		below[i] = append(below[i], n.Ref)
	}
	out := make([]Line, 0, len(plain)+len(notes))
	for i, l := range plain {
		out = append(out, l)
		for _, ref := range below[i] {
			n := l
			n.Fold, n.note = false, ref+1
			out = append(out, n)
		}
	}
	return out
}

func isNote(l Line) bool { return l.note > 0 }

// Note is the caller's Ref of a note line; ok is false for any other line.
func (l Line) Note() (ref int, ok bool) {
	return l.note - 1, l.note > 0
}

// NotesBelow is how many note lines follow line i.
func NotesBelow(lines []Line, i int) int {
	n := 0
	for j := i + 1; j < len(lines) && lines[j].note > 0; j++ {
		n++
	}
	return n
}

// anchorAt is the anchor of line i: the row and half of the line it is or
// follows, and how many note lines below that line it stands.
func anchorAt(lines []Line, i int) (row int, half Half, below int) {
	j := i
	for j > 0 && lines[j].note > 0 {
		j--
	}
	return RowAt(lines, j), lines[j].Half, i - j
}

// lineAt is the index of the line below notes of the line that shows half
// of row, as far as it has note lines.
func lineAt(lines []Line, row int, half Half, below int) int {
	i := LineFor(lines, row, half)
	return i + min(max(below, 0), NotesBelow(lines, i))
}
