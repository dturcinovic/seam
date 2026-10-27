package view

import (
	"sort"

	"github.com/dturcinovic/seam/internal/domain/change"
)

// Spot is what a line of a stream shows, and so what its anchor holds.
type Spot int

// The spots of a stream. The zero value is the header, so that StreamPos{}
// stands at the top of the stream.
const (
	// OnHead is the header line of a file.
	OnHead Spot = iota
	// OnRow is a line of a file that is laid out.
	OnRow
	// OnGuess is a line of a file that is not laid out yet, standing in for
	// lines of unknown rows.
	OnGuess
)

// StreamLine is one line of a stream: the header of File, or its line Line,
// an index into its lines with OnRow and into its estimate with OnGuess.
type StreamLine struct {
	File int
	Spot Spot
	Line int
}

// StreamPos is where a stream stands. File, Spot, AnchorRow, AnchorHalf
// and AnchorBelow are the anchor: with OnRow a row, its half and the note
// lines below it as in Pos, with OnGuess a line of the file's estimate,
// which Switch turns into a row once the file is laid out. Scroll is the
// index of the top line of the whole stream.
type StreamPos struct {
	File        int
	Spot        Spot
	AnchorRow   int
	AnchorHalf  Half
	AnchorBelow int
	Scroll      int
}

// StreamPosFor is the place in a stream of pos in file: its anchor on top,
// or the file's header if pos stands at the top of the file, so that the
// header shows what the single file view shows above its first line.
func StreamPosFor(file int, pos Pos) StreamPos {
	if pos.Scroll == 0 {
		return StreamPos{File: file}
	}
	return StreamPos{File: file, Spot: OnRow, AnchorRow: pos.AnchorRow, AnchorHalf: pos.AnchorHalf, AnchorBelow: pos.AnchorBelow}
}

// FilePos is the anchor of p in its file, for the single file view to
// Switch to. A header stands for the first row; a guess for the row of
// the same index, the best there is before the file is laid out.
func (p StreamPos) FilePos() Pos {
	if p.Spot == OnHead {
		return Pos{}
	}
	return Pos{AnchorRow: p.AnchorRow, AnchorHalf: p.AnchorHalf, AnchorBelow: p.AnchorBelow}
}

// Stream lays out the files of a changeset one below the other, each as a
// header line and then its lines, or an estimate of their number while it
// is not laid out. The zero value has no files.
type Stream struct {
	files []streamFile
	// ends[i] is the index of the line after file i, to find the file of a
	// line by binary search.
	ends []int
}

type streamFile struct {
	lines    []Line
	laid     bool
	estimate int
}

// NewStream is a stream of files none of which is laid out, file i with
// estimates[i] lines below its header; a negative estimate counts as none.
func NewStream(estimates []int) *Stream {
	s := &Stream{files: make([]streamFile, len(estimates)), ends: make([]int, len(estimates))}
	for i, e := range estimates {
		s.files[i].estimate = max(e, 0)
	}
	s.sum(0)
	return s
}

// Estimate is how many lines a file of counts is likely to show in mode m
// with context rows around each change. It may be wrong either way: without
// the unchanged rows the full view comes out short, and blocks that share
// their context make the hunk view shorter. A file git counts nothing for
// gets one line.
func Estimate(counts change.LineCounts, m Mode, context int) int {
	if !counts.Known {
		return 1
	}
	n := counts.Added + counts.Deleted
	if m == Hunks {
		n += 2 * max(context, 0)
	}
	return n
}

// Lay gives file its lines; a file index outside the stream is ignored.
func (s *Stream) Lay(file int, lines []Line) {
	if file < 0 || file >= len(s.files) {
		return
	}
	s.files[file] = streamFile{lines: lines, laid: true}
	s.sum(file)
}

// Forget drops the lines of file and keeps their number as its estimate,
// so that the stream keeps its length and the scroll bar its marks.
func (s *Stream) Forget(file int) {
	if file < 0 || file >= len(s.files) || !s.files[file].laid {
		return
	}
	s.files[file] = streamFile{estimate: s.files[file].body()}
}

// Files is the number of files in the stream.
func (s *Stream) Files() int {
	return len(s.files)
}

// Len is the number of lines in the stream.
func (s *Stream) Len() int {
	if len(s.ends) == 0 {
		return 0
	}
	return s.ends[len(s.ends)-1]
}

// Start is the index of the header line of file, which must be a file of
// the stream.
func (s *Stream) Start(file int) int {
	if file == 0 {
		return 0
	}
	return s.ends[file-1]
}

// End is the index of the line after file, which must be a file of the
// stream.
func (s *Stream) End(file int) int {
	return s.ends[file]
}

// Lines is the lines of file and whether it is laid out.
func (s *Stream) Lines(file int) ([]Line, bool) {
	if file < 0 || file >= len(s.files) {
		return nil, false
	}
	return s.files[file].lines, s.files[file].laid
}

// At is the line at index i of the stream, which must not be empty; an
// index outside it gives the nearest line.
func (s *Stream) At(i int) StreamLine {
	i = max(min(i, s.Len()-1), 0)
	file := sort.SearchInts(s.ends, i+1)
	line := i - s.Start(file) - 1
	switch {
	case line < 0:
		return StreamLine{File: file}
	case s.files[file].laid:
		return StreamLine{File: file, Spot: OnRow, Line: line}
	}
	return StreamLine{File: file, Spot: OnGuess, Line: line}
}

// Index is the index in the stream of l, the inverse of At.
func (s *Stream) Index(l StreamLine) int {
	if l.Spot == OnHead {
		return s.Start(l.File)
	}
	return s.Start(l.File) + 1 + l.Line
}

// Switch places a stream whose lines changed, after a file was laid out or
// forgotten, or a change of mode or context or layout: the anchor stays,
// and the top line becomes the one that holds it, within the bounds of the
// stream. A guess in a file that is laid out now becomes the row of its
// line. A file past the end gives the last one.
func (s *Stream) Switch(pos StreamPos, height int) StreamPos {
	if len(s.files) == 0 {
		return StreamPos{}
	}
	pos.File = max(min(pos.File, len(s.files)-1), 0)
	f := s.files[pos.File]
	switch {
	case pos.Spot == OnHead:
		pos.Scroll = s.Start(pos.File)
	case pos.Spot == OnGuess && f.laid && len(f.lines) > 0:
		i := min(pos.AnchorRow, len(f.lines)-1)
		pos.Spot = OnRow
		pos.AnchorRow, pos.AnchorHalf, pos.AnchorBelow = anchorAt(f.lines, i)
		pos.Scroll = s.Start(pos.File) + 1 + i
	case f.laid:
		// With no lines the header is all the file shows.
		pos.Scroll = s.Start(pos.File) + min(1+lineAt(f.lines, pos.AnchorRow, pos.AnchorHalf, pos.AnchorBelow), len(f.lines))
	default:
		// A row of a file not laid out, as after a reload: its index is the
		// best guess of its line.
		pos.Scroll = s.Start(pos.File) + min(1+pos.AnchorRow, f.body())
	}
	pos.Scroll = clampScroll(s.Len(), pos.Scroll, height)
	return pos
}

// Navigate moves the top line to scroll, within the bounds of the stream,
// and anchors the stream at the new top line; pos must be placed on this
// stream. As in Navigate of a single file, the anchor stays when nothing
// moved or a fold at the top holds it.
func (s *Stream) Navigate(pos StreamPos, scroll, height int) StreamPos {
	if s.Len() == 0 {
		pos.Scroll = 0
		return pos
	}
	scroll = clampScroll(s.Len(), scroll, height)
	if scroll == pos.Scroll {
		return pos
	}
	pos.Scroll = scroll
	top := s.At(scroll)
	switch top.Spot {
	case OnHead:
		pos.File, pos.Spot, pos.AnchorRow, pos.AnchorHalf, pos.AnchorBelow = top.File, OnHead, 0, Whole, 0
	case OnRow:
		lines := s.files[top.File].lines
		l := lines[top.Line]
		if l.Fold && top.File == pos.File && pos.Spot == OnRow && l.Contains(pos.AnchorRow) {
			// The fold itself is on top now, not a note line below it.
			pos.AnchorBelow = 0
			return pos
		}
		pos.File, pos.Spot = top.File, OnRow
		pos.AnchorRow, pos.AnchorHalf, pos.AnchorBelow = anchorAt(lines, top.Line)
	case OnGuess:
		pos.File, pos.Spot, pos.AnchorRow, pos.AnchorHalf, pos.AnchorBelow = top.File, OnGuess, top.Line, Whole, 0
	}
	return pos
}

// sum brings ends up to date from file on.
func (s *Stream) sum(file int) {
	end := s.Start(file)
	for i := file; i < len(s.files); i++ {
		end += 1 + s.files[i].body()
		s.ends[i] = end
	}
}

// body is the number of lines below the header.
func (f streamFile) body() int {
	if f.laid {
		return len(f.lines)
	}
	return f.estimate
}
