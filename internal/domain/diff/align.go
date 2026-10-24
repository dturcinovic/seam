package diff

import (
	"context"
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
// old and new lines; -1 means the side has no line in this row. A Change row
// may mark the words that differ in LeftInline and RightInline.
type Row struct {
	Kind                    RowKind
	Left, Right             int
	LeftInline, RightInline []Span
}

// RowRange is the rows from Start up to but not including End.
type RowRange struct {
	Start, End int
}

// FileDiff is two versions of a file aligned as rows. Changes lists the
// blocks of rows that differ, in order. Simplified reports that the time ran
// out: the diff is valid, but its blocks may be larger than needed and words
// left unmarked.
type FileDiff struct {
	Old, New   Side
	Rows       []Row
	Changes    []RowRange
	Simplified bool
}

// ApproxBytes is roughly the memory the diff holds: the lines with their
// headers, the rows and the spans. A cache weighs its entries with it.
func (d FileDiff) ApproxBytes() int64 {
	n := sideBytes(d.Old) + sideBytes(d.New)
	n += int64(cap(d.Rows)) * int64(unsafe.Sizeof(Row{}))
	for _, r := range d.Rows {
		n += int64(cap(r.LeftInline)+cap(r.RightInline)) * int64(unsafe.Sizeof(Span{}))
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
	marked := markInline(ctx, rows, old, new)
	return FileDiff{Old: old, New: new, Rows: rows, Changes: changes, Simplified: !complete || !marked}
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
