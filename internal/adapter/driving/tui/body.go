package tui

import (
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/lang"
	"github.com/dturcinovic/seam/internal/domain/view"
)

// The body scrolls through lines: those of the file shown, or in the stream
// those of every file, each below its header. The methods here tell the two
// apart for what a line shows, so that the caret, hovers and the panes work
// alike in both.

// bodyLine is what a line of the body shows: line of file's view with
// OnRow, or in the stream the file's header or with OnGuess the stand-in
// guess of its lines while it is not laid out.
type bodyLine struct {
	file  int
	spot  view.Spot
	line  view.Line
	guess int
}

// lineCount is how many lines the body scrolls through.
func (s *state) lineCount() int {
	if s.Stream.On {
		return s.Stream.View.Len()
	}
	return len(s.Lines)
}

// top is the index of the body's top line.
func (s *state) top() int {
	if s.Stream.On {
		return s.Stream.Pos.Scroll
	}
	return s.Pos.Scroll
}

// bodyAt is line i of the body; ok is false past either end.
func (s *state) bodyAt(i int) (bodyLine, bool) {
	if i < 0 || i >= s.lineCount() {
		return bodyLine{}, false
	}
	if !s.Stream.On {
		return bodyLine{file: s.Current, spot: view.OnRow, line: s.Lines[i]}, true
	}
	sl := s.Stream.View.At(i)
	b := bodyLine{file: sl.File, spot: sl.Spot, guess: sl.Line}
	if sl.Spot == view.OnRow {
		lines, _ := s.Stream.View.Lines(sl.File)
		b.line = lines[sl.Line]
	}
	return b, true
}

// rowLine is line i of the body if it shows rows of a file, not a note
// below them: the caret, hovers and the mouse find rows only.
func (s *state) rowLine(i int) (file int, l view.Line, ok bool) {
	b, ok := s.bodyAt(i)
	if !ok || b.spot != view.OnRow {
		return 0, view.Line{}, false
	}
	if _, isNote := b.line.Note(); isNote {
		return 0, view.Line{}, false
	}
	return b.file, b.line, true
}

// bodyFiles is the files the body shows, from first to before end: every
// file in the stream, the current one alone else.
func (s *state) bodyFiles() (first, end int) {
	if s.Stream.On {
		return 0, len(s.Files)
	}
	return s.Current, s.Current + 1
}

// linesOf is the view of file, nil if it is not laid out.
func (s *state) linesOf(file int) []view.Line {
	if s.Stream.On {
		lines, _ := s.Stream.View.Lines(file)
		return lines
	}
	if file != s.Current {
		return nil
	}
	return s.Lines
}

// bodyIndex is the index in the body of line i of file's view.
func (s *state) bodyIndex(file, i int) int {
	if s.Stream.On {
		return s.Stream.View.Index(view.StreamLine{File: file, Spot: view.OnRow, Line: i})
	}
	return i
}

// rowIndex is the line of the body nearest row of file: the line that
// shows or folds it, or the file's header while it is not laid out.
func (s *state) rowIndex(file, row int) int {
	if lines := s.linesOf(file); len(lines) > 0 {
		return s.bodyIndex(file, view.LineForRow(lines, row))
	}
	if s.Stream.On && file >= 0 && file < s.Stream.View.Files() {
		return s.Stream.View.Start(file)
	}
	return 0
}

// loadedOf is the loaded diff of file; ok is false before it arrived.
func (s *state) loadedOf(file int) (*filediff.Snapshot, bool) {
	if s.Stream.On {
		if h, ok := s.Stream.Held[file]; ok {
			return &h.snap, h.err == nil
		}
		return &filediff.Snapshot{}, false
	}
	if file != s.Current {
		return &filediff.Snapshot{}, false
	}
	return &s.Snapshot, s.Loaded
}

// loadErr is why file failed to load, nil while it has not.
func (s *state) loadErr(file int) error {
	if !s.Stream.On {
		return s.Err
	}
	if h, ok := s.Stream.Held[file]; ok {
		return h.err
	}
	return nil
}

// snapOf is the loaded diff of file, empty if it has none.
func (s *state) snapOf(file int) *filediff.Snapshot {
	snap, _ := s.loadedOf(file)
	return snap
}

// layoutOf is the layout of file's lines: the screen's, with gutters as
// wide as file's line numbers need, as each file of the stream has its own.
func (s *state) layoutOf(file int) layout {
	l := s.Layout
	if h, ok := s.Stream.Held[file]; s.Stream.On && ok {
		l.Left.Digits, l.Right.Digits = h.left, h.right
	}
	return l
}

// instanceOf is the language server instance that serves side w of file.
func (s *state) instanceOf(file int, w filediff.Which) lang.Instance {
	if !s.Stream.On {
		return s.LSP.shown(w)
	}
	if h, ok := s.Stream.Held[file]; ok {
		return h.servers.shown(w)
	}
	return lang.Instance{}
}
