package note

import (
	"github.com/dturcinovic/seam/internal/domain/change"
)

// Where is how surely a note was found again.
type Where uint8

// The outcomes of Locate. The zero value is no outcome.
const (
	_ Where = iota
	// Exact is the selection with its full context at exactly one place,
	// of a selection that stood once.
	Exact
	// Likely is the best guess, but no proof: the one place with at least
	// half the context, or the copy by its order among as many as there
	// were.
	Likely
	// Changed is the lines between the context before and after, each
	// found once: the selection was changed within.
	Changed
	// Ambiguous is more than one place, none better than the others.
	Ambiguous
	// Orphaned is no place.
	Orphaned
)

// String names the outcome for the user.
func (w Where) String() string {
	switch w {
	case Exact:
		return "placed"
	case Likely:
		return "likely placed"
	case Changed:
		return "placed, changed"
	case Ambiguous:
		return "ambiguous"
	case Orphaned:
		return "orphaned"
	}
	return "unset"
}

// Shown reports that the outcome has a place to show the note at.
func (w Where) Shown() bool { return w == Exact || w == Likely || w == Changed }

// Placement is where a note stands in a version of its file: Start to End,
// 1-based and End included, when Where is Shown.
type Placement struct {
	Where      Where
	Start, End int
}

// Locate finds a's place in lines, the version blob of a's file. In the
// version it was set in it stands where it was set. Elsewhere it is a best
// guess from the text around it, no proof of identity: a block pasted in
// with the same selection and context cannot be told from the original.
// Rather than guess between places, it answers Ambiguous.
func Locate(a Anchor, blob change.ObjectID, lines []string) Placement {
	if !blob.IsZero() && blob == a.Blob && a.End <= len(lines) {
		return Placement{Where: Exact, Start: a.Start, End: a.End}
	}
	cands := a.candidates(lines)
	if len(cands) == 0 {
		return a.changed(lines)
	}
	if a.Copies > 1 {
		return a.byOrdinal(lines, cands)
	}
	full := a.fullScore()
	best, at, ties := -1, 0, 0
	for _, p := range cands {
		switch s := a.score(lines, p); {
		case s > best:
			best, at, ties = s, p, 1
		case s == best:
			ties++
		}
	}
	switch {
	case 2*best < full:
		// The same text elsewhere, without what was around it, is no
		// trace of the selection; its context may still bracket it.
		return a.changed(lines)
	case ties > 1:
		return Placement{Where: Ambiguous}
	case best == full:
		return a.at(Exact, at)
	}
	return a.at(Likely, at)
}

// byOrdinal places a selection that stood more than once: its copy by
// order, while there are as many as there were, and only as Likely, since
// blocks moved about keep no order.
func (a Anchor) byOrdinal(lines []string, cands []int) Placement {
	var full []int
	for _, p := range cands {
		if a.score(lines, p) == a.fullScore() {
			full = append(full, p)
		}
	}
	if len(full) != a.Copies || a.Ordinal < 1 || a.Ordinal > len(full) {
		return Placement{Where: Ambiguous}
	}
	return a.at(Likely, full[a.Ordinal-1])
}

// changed places a selection not found where it was between its context: Before
// and After each found once, in order, with at most MaxLines lines and at
// least one between them. Context that is empty, as at the file's start or
// end, stands there.
func (a Anchor) changed(lines []string) Placement {
	if len(a.Before) == 0 && len(a.After) == 0 {
		return Placement{Where: Orphaned}
	}
	from, ok := once(lines, a.Before, 0)
	if !ok {
		return Placement{Where: Orphaned}
	}
	from += len(a.Before)
	to, ok := once(lines, a.After, len(lines))
	if !ok || to <= from || to-from > MaxLines {
		return Placement{Where: Orphaned}
	}
	return Placement{Where: Changed, Start: from + 1, End: to}
}

// once is where block stands in lines if it stands there exactly once;
// an empty block stands at empty.
func once(lines, block []string, empty int) (int, bool) {
	if len(block) == 0 {
		return empty, true
	}
	at := positions(lines, block)
	if len(at) != 1 {
		return 0, false
	}
	return at[0], true
}

func (a Anchor) at(w Where, p int) Placement {
	return Placement{Where: w, Start: p + 1, End: p + len(a.Lines)}
}
