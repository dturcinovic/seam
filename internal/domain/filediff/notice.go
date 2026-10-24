package filediff

import (
	"errors"

	"seam/internal/domain/change"
)

// NoticeKind is what a notice reports.
type NoticeKind int

// The kinds of notice. The zero value is no valid kind.
const (
	_ NoticeKind = iota
	// TooLarge means a side exceeds the file size limit and was not read.
	TooLarge
	// Binary means a side holds binary data and is not shown as text.
	Binary
	// Simplified means the time for the diff ran out: its blocks may be
	// larger than needed. The UI learns it here, not from the diff.
	Simplified
	// HighlightStopped means a side has syntax colours only up to Line,
	// the first line without: a line was too long or the time ran out.
	HighlightStopped
)

// Which names one side of the file.
type Which int

// The sides. The zero value means the notice is about the whole file.
const (
	_ Which = iota
	// OldSide is the left side.
	OldSide
	// NewSide is the right side.
	NewSide
)

// Notice tells why a snapshot shows less than the file. Size is set for
// TooLarge: the size known when reading stopped. Line is set for
// HighlightStopped, counted from 1.
type Notice struct {
	Kind NoticeKind
	Side Which
	Size int64
	Line int
}

// SideNotice judges what reading a side returned. A side too large or
// binary is a notice, as the file can still be named and listed; any other
// error fails the load.
func SideNotice(side Which, data []byte, err error) (*Notice, error) {
	if tooLarge, ok := errors.AsType[*change.TooLargeError](err); ok {
		return &Notice{Kind: TooLarge, Side: side, Size: tooLarge.Size}, nil
	}
	if err != nil {
		return nil, err
	}
	if change.IsBinary(data) {
		return &Notice{Kind: Binary, Side: side}, nil
	}
	return nil, nil
}
