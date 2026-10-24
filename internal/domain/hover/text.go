package hover

import (
	"errors"

	"seam/internal/domain/lang"
	"seam/internal/domain/syntax"
)

// ErrColourerFailed is why answers with code come uncoloured once
// colouring them has failed.
var ErrColourerFailed = errors.New("hover colourer failed")

// MaxText is the most of an answer's text, in bytes, that is read: past
// it the rest is cut, which keeps the work per answer small.
const MaxText = 32 << 10

// Content is an answer as blocks to show one after the other. Cut says the
// answer was longer than MaxText and lost its end.
type Content struct {
	Blocks []Block
	Cut    bool
}

// BlockKind tells which fields of a Block are set.
type BlockKind uint8

const (
	// Paragraph sets Spans: text that flows to the width it is shown in.
	Paragraph BlockKind = iota + 1
	// Heading sets Spans, like Paragraph.
	Heading
	// Code sets Code, its language Lang if known, and, once coloured,
	// Tokens; its lines are kept as they are.
	Code
	// Rule is a line across, between parts of the answer.
	Rule
)

// Block is a part of an answer.
type Block struct {
	Kind   BlockKind
	Spans  []Span
	Lang   lang.Lang
	Code   string
	Tokens syntax.Tokens
}

// SpanKind is how a stretch of a paragraph stands out. Where one is
// nested in another, the later kind shows: code within a link's text
// stays code.
type SpanKind uint8

// The kinds of span; Plain is text that does not stand out.
const (
	Plain SpanKind = iota
	Emphasis
	Strong
	// Link is the text of a link, URL its target.
	Link
	URL
	// InlineCode is code within the text.
	InlineCode
)

// Span is a stretch of a paragraph. A newline in Text breaks the line
// there; elsewhere a paragraph breaks where it fits.
type Span struct {
	Kind SpanKind
	Text string
}

// HasCode says that some block is code in a known language, which a
// highlighter can colour.
func (c Content) HasCode() bool {
	for _, b := range c.Blocks {
		if b.Kind == Code && b.Lang != "" {
			return true
		}
	}
	return false
}
