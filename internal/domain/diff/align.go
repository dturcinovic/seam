package diff

import (
	"cmp"
	"context"
	"slices"
	"unsafe"
)

// RowKind says how the lines of a row relate.
type RowKind int

// The kinds of row. The zero value is no valid kind.
const (
	_ RowKind = iota
	// Equal pairs two equal lines.
	Equal
	// Delete has an old line only.
	Delete
	// Insert has a new line only.
	Insert
	// Change pairs an old and a new line of the same changed block.
	Change
)

// Row is one line of the side-by-side view. Left and Right index into the
// old and new lines; -1 means the side has no line in this row.
type Row struct {
	Kind        RowKind
	Left, Right int
}

// InlineSpans marks the words that differ in the two lines of a Change row.
// They live beside the rows, not in them: most rows have none, and a cache
// holds every row of every file it keeps.
type InlineSpans struct {
	Row         int
	Left, Right []Span
}

// RowRange is the rows from Start up to but not including End.
type RowRange struct {
	Start, End int
}

// FileDiff is two versions of a file aligned as rows. Changes lists the
// blocks of rows that differ, in order. Inline holds one entry per marked
// Change row, ordered by Row, which InlineAt relies on. Simplified reports that the time ran out: the diff is valid, but its
// blocks may be larger than needed and words left unmarked.
type FileDiff struct {
	Old, New   Side
	Rows       []Row
	Changes    []RowRange
	Inline     []InlineSpans
	Simplified bool
}

// InlineAt returns the marked words of a row; none for a row without marks.
func (d FileDiff) InlineAt(row int) (left, right []Span) {
	i, ok := slices.BinarySearchFunc(d.Inline, row, func(s InlineSpans, row int) int { return cmp.Compare(s.Row, row) })
	if !ok {
		return nil, nil
	}
	return d.Inline[i].Left, d.Inline[i].Right
}

// ApproxBytes is roughly the memory the diff holds: the lines with their
// headers, the rows and the spans. A cache weighs its entries with it.
func (d FileDiff) ApproxBytes() int64 {
	n := sideBytes(d.Old) + sideBytes(d.New)
	n += int64(cap(d.Rows)) * int64(unsafe.Sizeof(Row{}))
	n += int64(cap(d.Inline)) * int64(unsafe.Sizeof(InlineSpans{}))
	for _, s := range d.Inline {
		n += int64(cap(s.Left)+cap(s.Right)) * int64(unsafe.Sizeof(Span{}))
	}
	return n + int64(cap(d.Changes))*int64(unsafe.Sizeof(RowRange{}))
}

func sideBytes(s Side) int64 {
	n := int64(cap(s.Lines)) * int64(unsafe.Sizeof(""))
	for _, l := range s.Lines {
		n += int64(len(l))
	}
	return n
}

// Compute aligns old and new. When ctx ends, it stops searching and returns
// the rest of the files as changed blocks rather than fail.
func Compute(ctx context.Context, old, new Side) FileDiff {
	a, b := intern(old.Lines, new.Lines)
	matches, complete := commonSubsequence(ctx, a, b)
	rows, changes := align(matches, len(a), len(b))
	inline, marked := markInline(ctx, rows, old, new)
	return FileDiff{Old: old, New: new, Rows: rows, Changes: changes, Inline: inline, Simplified: !complete || !marked}
}

// intern maps each distinct old line or token to a number, so the search
// compares ints. A new one that is not among the old can match nothing and
// gets noMatch.
func intern(old, new []string) (a, b []int) {
	ids := make(map[string]int, len(old))
	a, b = make([]int, len(old)), make([]int, len(new))
	for i, line := range old {
		n, ok := ids[line]
		if !ok {
			n = len(ids)
			ids[line] = n
		}
		a[i] = n
	}
	for i, line := range new {
		if n, ok := ids[line]; ok {
			b[i] = n
		} else {
			b[i] = noMatch
		}
	}
	return a, b
}

// noMatch is the id of a new line or token that occurs nowhere in the old.
const noMatch = -1

// align turns the matched pairs into rows. Between two matches lies a
// changed block of a old and b new lines: its first min(a, b) lines pair up
// as Change, the rest are Delete or Insert.
func align(matches []match, nOld, nNew int) ([]Row, []RowRange) {
	rows := make([]Row, 0, rowCount(matches, nOld, nNew))
	var changes []RowRange
	i, j := 0, 0
	block := func(iEnd, jEnd int) {
		if i == iEnd && j == jEnd {
			return
		}
		start := len(rows)
		paired := min(iEnd-i, jEnd-j)
		for k := range paired {
			rows = append(rows, Row{Kind: Change, Left: i + k, Right: j + k})
		}
		for k := i + paired; k < iEnd; k++ {
			rows = append(rows, Row{Kind: Delete, Left: k, Right: -1})
		}
		for k := j + paired; k < jEnd; k++ {
			rows = append(rows, Row{Kind: Insert, Left: -1, Right: k})
		}
		changes = append(changes, RowRange{Start: start, End: len(rows)})
	}
	for _, m := range matches {
		block(m.old, m.new)
		rows = append(rows, Row{Kind: Equal, Left: m.old, Right: m.new})
		i, j = m.old+1, m.new+1
	}
	block(nOld, nNew)
	return rows, changes
}

// rowCount is the exact number of rows: the FileDiff keeps them, so spare
// capacity would be memory held for nothing.
func rowCount(matches []match, nOld, nNew int) int {
	n, i, j := 0, 0, 0
	for _, m := range matches {
		n += max(m.old-i, m.new-j) + 1
		i, j = m.old+1, m.new+1
	}
	return n + max(nOld-i, nNew-j)
}
