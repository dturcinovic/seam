package note

import (
	"fmt"
	"slices"

	"github.com/dturcinovic/seam/internal/domain/change"
)

// Anchor is where a note is set: the lines Start to End of Path, 1-based
// and End included, in the version Blob of Origin. Lines is the selection
// as it stood, kept unchanged for the agent; Before and After are up to
// three lines around it, to find it again. Copies is how often selection
// and context stood in that version, and Ordinal which of them is meant.
type Anchor struct {
	Path            string
	Origin          Origin
	Blob            change.ObjectID
	Start, End      int
	Lines           []string
	Before, After   []string
	Copies, Ordinal int
}

// NewAnchor anchors the lines start to end of lines, the version blob of
// path in origin.
func NewAnchor(path string, origin Origin, blob change.ObjectID, lines []string, start, end int) (Anchor, error) {
	if start < 1 || end < start || end > len(lines) {
		return Anchor{}, fmt.Errorf("%w: lines %d to %d of %d", ErrInvalidRange, start, end, len(lines))
	}
	if n := end - start + 1; n > MaxLines {
		return Anchor{}, fmt.Errorf("%w: %d lines, at most %d", ErrTooManyLines, n, MaxLines)
	}
	a := Anchor{
		Path: path, Origin: origin, Blob: blob, Start: start, End: end,
		Lines:  slices.Clone(lines[start-1 : end]),
		Before: slices.Clone(lines[max(start-1-contextLines, 0) : start-1]),
		After:  slices.Clone(lines[end:min(end+contextLines, len(lines))]),
	}
	for _, p := range a.candidates(lines) {
		if a.score(lines, p) == a.fullScore() {
			a.Copies++
			if p == start-1 {
				a.Ordinal = a.Copies
			}
		}
	}
	return a, nil
}

// candidates are the 0-based lines of lines where the selection stands.
func (a Anchor) candidates(lines []string) []int { return positions(lines, a.Lines) }

// positions are the 0-based lines of lines where block stands.
func positions(lines, block []string) []int {
	var at []int
	for p := 0; p+len(block) <= len(lines); p++ {
		if slices.Equal(lines[p:p+len(block)], block) {
			at = append(at, p)
		}
	}
	return at
}

// score is how many lines of the context stand at their place around the
// selection at p; a line changed in between costs only itself.
func (a Anchor) score(lines []string, p int) int {
	n := 0
	for i, l := range a.Before {
		if at := p - len(a.Before) + i; at >= 0 && lines[at] == l {
			n++
		}
	}
	for i, l := range a.After {
		if at := p + len(a.Lines) + i; at < len(lines) && lines[at] == l {
			n++
		}
	}
	return n
}

func (a Anchor) fullScore() int { return len(a.Before) + len(a.After) }
