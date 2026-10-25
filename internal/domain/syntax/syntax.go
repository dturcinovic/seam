package syntax

import (
	"fmt"
	"slices"
	"strings"
	"unsafe"

	"github.com/dturcinovic/seam/internal/domain/diff"
)

// The bounds on long lines, in bytes without the line ending. A lexer reads
// a side only up to its first line longer than MaxLine, see Lexable. A line
// longer than MaxRunsLine is still read, so the lines after it start in the
// right state, but keeps no runs, see Builder.
const (
	MaxLine     = 64 << 10
	MaxRunsLine = 4 << 10
)

// Lexable returns text up to its first line over MaxLine: a lexer's time
// and memory grow with what it is given, and it must not reach that line
// at all, even with a token that would span it.
func Lexable(text string) string {
	at := 0
	for line := range strings.Lines(text) {
		if len(diff.Content(line)) > MaxLine {
			return text[:at]
		}
		at += len(line)
	}
	return text
}

// Kind is what a stretch of source is to the language. The zero value,
// Plain, is text without a kind of its own.
type Kind uint8

// The kinds of token.
const (
	Plain Kind = iota
	Keyword
	// Type is a type name, built in or declared.
	Type
	Function
	// Builtin is a predeclared name other than a type, such as len.
	Builtin
	// Constant is a predeclared or declared constant, such as nil or true.
	Constant
	String
	// Escape is an escape sequence within a string.
	Escape
	Number
	Comment
	// Meta is a directive to the compiler or template rather than code,
	// such as a preprocessor line or <?php.
	Meta
	Operator
	Punctuation
	// Tag is the name of a markup tag, Attribute that of one of its
	// attributes.
	Tag
	Attribute
	Variable
	// Kinds is the number of kinds, no kind itself.
	Kinds
)

// Run is a stretch of a line in one kind. It reaches from the previous
// run's End, or the line's start, to End, in bytes from the line's start;
// int32 halves what a file's runs hold, and lines with runs are short.
type Run struct {
	End  int32
	Kind Kind
}

// Tokens are the runs of a side's first Lines() lines; a line past them has
// no kinds, nor has a line without runs or the rest of a line past its last
// run. The zero value covers no line.
type Tokens struct {
	// runs holds all lines' runs one after the other; line i's end at
	// ends[i], so a side costs two allocations, not one per line.
	runs []Run
	ends []int32
}

// Lines is how many lines from the first the tokens cover.
func (t Tokens) Lines() int { return len(t.ends) }

// Line returns the runs of line i, nil past the lines covered. The caller
// must not change them.
func (t Tokens) Line(i int) []Run {
	if i < 0 || i >= len(t.ends) {
		return nil
	}
	start := int32(0)
	if i > 0 {
		start = t.ends[i-1]
	}
	end := t.ends[i]
	return t.runs[start:end:end]
}

// ApproxBytes is roughly the memory the tokens hold.
func (t Tokens) ApproxBytes() int64 {
	return int64(cap(t.runs))*int64(unsafe.Sizeof(Run{})) + int64(cap(t.ends))*int64(unsafe.Sizeof(int32(0)))
}

// Builder builds Tokens line by line. Adjacent runs of one kind merge, a
// line's trailing plain run is dropped, as past the last run is plain
// anyway, and so are all runs of a line that reaches past MaxRunsLine.
type Builder struct {
	t Tokens
	// at is where the current line's next run starts.
	at int
}

// Add ends the current run of the current line at end with kind k. A run
// that ends where the last one did is empty and ignored.
func (b *Builder) Add(end int, k Kind) error {
	if k >= Kinds {
		return fmt.Errorf("syntax: no kind %d", k)
	}
	if end < b.at {
		return fmt.Errorf("syntax: run ends at %d before its start %d", end, b.at)
	}
	if end == b.at {
		return nil
	}
	if end > MaxLine+len("\r\n") {
		return fmt.Errorf("syntax: run ends at %d past the longest line", end)
	}
	b.at = end
	if n := len(b.t.runs); n > b.lineStart() && b.t.runs[n-1].Kind == k {
		b.t.runs[n-1].End = int32(end)
		return nil
	}
	b.t.runs = append(b.t.runs, Run{End: int32(end), Kind: k})
	return nil
}

// EndLine ends the current line, of length bytes without its ending; the
// next Add starts the next one.
func (b *Builder) EndLine(length int) {
	start := b.lineStart()
	if length > MaxRunsLine {
		b.t.runs = b.t.runs[:start]
	}
	if n := len(b.t.runs); n > start && b.t.runs[n-1].Kind == Plain {
		b.t.runs = b.t.runs[:n-1]
	}
	b.t.ends = append(b.t.ends, int32(len(b.t.runs)))
	b.at = 0
}

// Lines is how many lines have been ended.
func (b *Builder) Lines() int { return len(b.t.ends) }

// Tokens returns the lines ended so far, copied to their size, as they may
// be kept long. The builder must not be used after.
func (b *Builder) Tokens() Tokens {
	return Tokens{runs: slices.Clone(b.t.runs[:b.lineStart()]), ends: slices.Clone(b.t.ends)}
}

func (b *Builder) lineStart() int {
	if len(b.t.ends) == 0 {
		return 0
	}
	return int(b.t.ends[len(b.t.ends)-1])
}
