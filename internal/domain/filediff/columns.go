package filediff

import (
	"sort"
	"unsafe"
)

// Checkpoint is a grapheme boundary in a long line where counting columns
// can start instead of at the line's beginning. Byte is its offset in the
// line's content. Col is its column as terminals count by default, a
// grapheme's code points each; GraphemeCol as they count in Unicode core
// mode, a grapheme as one. The terminal decides which applies.
type Checkpoint struct {
	Byte, Col, GraphemeCol int
}

// LineCheckpoints are the checkpoints of one line of a side, in order.
type LineCheckpoints struct {
	Line   int
	Points []Checkpoint
}

// ColumnIndex holds the checkpoints of the long lines of one side, ordered
// by line. Lines without any start counting at their beginning.
type ColumnIndex struct {
	Lines []LineCheckpoints
}

// Columns are the column indexes of both sides.
type Columns struct {
	Old, New ColumnIndex
}

// At returns the checkpoints of line, or nil if it has none.
func (c ColumnIndex) At(line int) []Checkpoint {
	i := sort.Search(len(c.Lines), func(i int) bool { return c.Lines[i].Line >= line })
	if i < len(c.Lines) && c.Lines[i].Line == line {
		return c.Lines[i].Points
	}
	return nil
}

// ApproxBytes is roughly the memory both indexes hold.
func (c Columns) ApproxBytes() int64 {
	return c.Old.approxBytes() + c.New.approxBytes()
}

func (c ColumnIndex) approxBytes() int64 {
	n := int64(cap(c.Lines)) * int64(unsafe.Sizeof(LineCheckpoints{}))
	for _, l := range c.Lines {
		n += int64(cap(l.Points)) * int64(unsafe.Sizeof(Checkpoint{}))
	}
	return n
}
