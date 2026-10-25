package tui

import (
	"fmt"
	"strings"

	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/syntax"
	"github.com/dturcinovic/seam/internal/textcell"
)

// cutHint closes the text of an answer that was cut.
func cutHint() string {
	return fmt.Sprintf("… cut at %d KiB", hover.MaxText>>10)
}

// popupLine is a line of the popup's text, styled by Runs, or a Rule
// across the popup.
type popupLine struct {
	Text string
	Runs []styledRun
	Rule bool
}

// styledRun is a stretch of a line in one style. It reaches from the
// previous run's End, or the line's start, to End, in bytes; past the last
// run the line is in popupStyle.
type styledRun struct {
	End int
	St  style
}

// popupText lays c out in lines of at most width columns and returns them
// with the widest's columns. Blocks are a blank line apart, but around a
// rule.
func popupText(c hover.Content, width int, m textcell.Method) (lines []popupLine, widest int) {
	add := func(ls []popupLine) {
		for _, l := range ls {
			widest = max(widest, textWidth(l.Text, m))
		}
		lines = append(lines, ls...)
	}
	for i, b := range c.Blocks {
		if i > 0 && b.Kind != hover.Rule && c.Blocks[i-1].Kind != hover.Rule {
			lines = append(lines, popupLine{})
		}
		switch b.Kind {
		case hover.Rule:
			lines = append(lines, popupLine{Rule: true})
		case hover.Code:
			for j, code := range strings.Split(b.Code, "\n") {
				add(wrapLine(codeText(code, b.Tokens.Line(j)), width, m, false))
			}
		case hover.Paragraph, hover.Heading:
			for _, l := range spanLines(b.Spans, b.Kind == hover.Heading) {
				add(wrapLine(l, width, m, true))
			}
		}
	}
	if c.Cut {
		if len(lines) > 0 {
			lines = append(lines, popupLine{})
		}
		var lb lineBuilder
		lb.add(cutHint(), popupDimStyle)
		add(wrapLine(lb.line(), width, m, true))
	}
	return lines, widest
}

// codeText is a line of code coloured by its runs.
func codeText(text string, runs []syntax.Run) popupLine {
	var lb lineBuilder
	at := 0
	for _, r := range runs {
		end := min(int(r.End), len(text))
		lb.add(text[at:end], popupCodeStyle(r.Kind))
		at = end
	}
	lb.add(text[at:], popupStyle)
	return lb.line()
}

// spanLines are the lines of a paragraph, broken where its text holds a
// newline; a heading's plain text is strong.
func spanLines(spans []hover.Span, heading bool) []popupLine {
	var lines []popupLine
	var lb lineBuilder
	for _, s := range spans {
		st := spanStyle(s.Kind, heading)
		for i, part := range strings.Split(s.Text, "\n") {
			if i > 0 {
				lines = append(lines, lb.line())
			}
			lb.add(part, st)
		}
	}
	return append(lines, lb.line())
}

// lineBuilder builds a popup line piece by piece, each tab as popupTab:
// the popup's lines start anywhere, so tab stops would not line up.
type lineBuilder struct {
	b    strings.Builder
	runs []styledRun
}

func (lb *lineBuilder) add(s string, st style) {
	if s == "" {
		return
	}
	lb.b.WriteString(strings.ReplaceAll(s, "\t", popupTab))
	lb.runs = append(lb.runs, styledRun{lb.b.Len(), st})
}

// line returns the line built and starts the next.
func (lb *lineBuilder) line() popupLine {
	l := popupLine{Text: lb.b.String(), Runs: lb.runs}
	lb.b.Reset()
	lb.runs = nil
	return l
}

func spanStyle(k hover.SpanKind, heading bool) style {
	switch k {
	case hover.Emphasis:
		return popupEmphasisStyle
	case hover.Strong:
		return popupStrongStyle
	case hover.Link:
		return popupLinkStyle
	case hover.URL:
		return popupDimStyle
	case hover.InlineCode:
		return popupCodeSpanStyle
	case hover.Plain:
	}
	if heading {
		return popupStrongStyle
	}
	return popupStyle
}

// wrapLine breaks l into lines of at most width columns at grapheme
// boundaries; with words, after the last blank that leaves the line
// within width where there is one, the blanks at the break dropped.
func wrapLine(l popupLine, width int, m textcell.Method, words bool) []popupLine {
	var out []popupLine
	start, startCol := 0, 0
	// word is where the word after the last blank starts, at column wordCol.
	word, wordCol := 0, 0
	afterBlank := false
	cells := textcell.New(l.Text, m)
	for c, ok := cells.Next(); ok; c, ok = cells.Next() {
		blank := words && l.Text[c.Start:c.End] == " "
		if words && afterBlank && !blank {
			word, wordCol = c.Start, c.Col
		}
		afterBlank = blank
		// A second round breaks within the word, if it is too long itself.
		for !blank && c.Col+c.Width-startCol > width && c.Start > start {
			end, col := c.Start, c.Col
			if word > start {
				end, col = word, wordCol
			}
			out = append(out, l.slice(start, end, words))
			start, startCol = end, col
		}
	}
	return append(out, l.slice(start, len(l.Text), words))
}

// slice is the part of l from start to end, less its blanks at the end if
// words.
func (l popupLine) slice(start, end int, words bool) popupLine {
	if words {
		end = start + len(strings.TrimRight(l.Text[start:end], " "))
	}
	s := popupLine{Text: l.Text[start:end]}
	for _, r := range l.Runs {
		if r.End <= start {
			continue
		}
		s.Runs = append(s.Runs, styledRun{min(r.End, end) - start, r.St})
		if r.End >= end {
			break
		}
	}
	return s
}
