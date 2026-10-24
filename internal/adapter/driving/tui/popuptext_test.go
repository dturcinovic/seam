package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
	"seam/internal/domain/syntax"
	"seam/internal/textcell"
)

// segment is a stretch of a popup line in one style.
type segment struct {
	text string
	st   style
}

// segments splits l by the style of each byte, as cellStyle picks it.
func segments(l popupLine) []segment {
	var segs []segment
	cs := cellStyle{fixed: popupStyle, runs: l.Runs}
	cells := textcell.New(l.Text, textcell.Graphemes)
	for c, ok := cells.Next(); ok; c, ok = cells.Next() {
		st := cs.of(c)
		if n := len(segs); n > 0 && segs[n-1].st == st {
			segs[n-1].text += l.Text[c.Start:c.End]
			continue
		}
		segs = append(segs, segment{l.Text[c.Start:c.End], st})
	}
	return segs
}

func para(spans ...hover.Span) hover.Block {
	return hover.Block{Kind: hover.Paragraph, Spans: spans}
}

func TestParagraphsBreakAtBlanks(t *testing.T) {
	c := hover.Content{Blocks: []hover.Block{para(hover.Span{Text: "Println formats   using the default formats\nfor its operands"})}}

	lines, widest := popupText(c, 12, textcell.Graphemes)

	assert.Equal(t, []string{"Println", "formats", "using the", "default", "formats", "for its", "operands"}, texts(popup{Lines: lines}))
	assert.Equal(t, 9, widest)
}

func TestAWordTooLongBreaksWithin(t *testing.T) {
	c := hover.Content{Blocks: []hover.Block{para(hover.Span{Text: "see https://pkg.go.dev/fmt#Println now"})}}

	lines, _ := popupText(c, 10, textcell.Graphemes)

	assert.Equal(t, []string{"see", "https://pk", "g.go.dev/f", "mt#Println", "now"}, texts(popup{Lines: lines}))
}

func TestBlocksAreSpacedButAroundRules(t *testing.T) {
	c := hover.Content{Blocks: []hover.Block{
		{Kind: hover.Code, Lang: lang.Go, Code: "type T int"},
		{Kind: hover.Rule},
		para(hover.Span{Text: "T is a type."}),
		{Kind: hover.Heading, Spans: []hover.Span{{Text: "Use"}}},
		para(hover.Span{Text: "Often."}),
	}, Cut: true}

	lines, widest := popupText(c, 40, textcell.Graphemes)

	assert.Equal(t, []string{"type T int", "", "T is a type.", "", "Use", "", "Often.", "", cutHint()}, texts(popup{Lines: lines}))
	assert.True(t, lines[1].Rule)
	assert.Equal(t, textWidth(cutHint(), textcell.Graphemes), widest)
	assert.Equal(t, []segment{{"Use", popupStrongStyle}}, segments(lines[4]))
	assert.Equal(t, []segment{{cutHint(), popupDimStyle}}, segments(lines[8]))
}

func TestSpansKeepTheirStylesAcrossBreaks(t *testing.T) {
	c := hover.Content{Blocks: []hover.Block{para(
		hover.Span{Kind: hover.Emphasis, Text: "@param"},
		hover.Span{Text: " "},
		hover.Span{Kind: hover.InlineCode, Text: "a b"},
		hover.Span{Text: " see "},
		hover.Span{Kind: hover.Link, Text: "docs"},
		hover.Span{Text: " "},
		hover.Span{Kind: hover.URL, Text: "https://x.org"},
		hover.Span{Kind: hover.Strong, Text: "\nsum"},
	)}}

	lines, _ := popupText(c, 13, textcell.Graphemes)

	require.Len(t, lines, 4)
	assert.Equal(t, []segment{{"@param", popupEmphasisStyle}, {" ", popupStyle}, {"a b", popupCodeSpanStyle}}, segments(lines[0]))
	assert.Equal(t, []segment{{"see ", popupStyle}, {"docs", popupLinkStyle}}, segments(lines[1]))
	assert.Equal(t, []segment{{"https://x.org", popupDimStyle}}, segments(lines[2]))
	assert.Equal(t, []segment{{"sum", popupStrongStyle}}, segments(lines[3]))
}

func TestCodeIsColouredByItsTokens(t *testing.T) {
	tok := tokens(t, []run{{1, syntax.Plain}, {5, syntax.Keyword}, {6, syntax.Plain}, {7, syntax.Type}}, []run{{1, syntax.Number}})
	c := hover.Content{Blocks: []hover.Block{{Kind: hover.Code, Lang: lang.Go, Code: "\tfunc T()\nx", Tokens: tok}}}

	lines, _ := popupText(c, 40, textcell.Graphemes)

	require.Len(t, lines, 2)
	assert.Equal(t, []segment{
		{"    ", popupCodeStyle(syntax.Plain)}, {"func", popupCodeStyle(syntax.Keyword)}, {" ", popupCodeStyle(syntax.Plain)},
		{"T", popupCodeStyle(syntax.Type)}, {"()", popupStyle},
	}, segments(lines[0]), "the tab expanded, the colours moved along")
	assert.Equal(t, []segment{{"x", popupCodeStyle(syntax.Number)}}, segments(lines[1]))
}

func TestARuleSpansThePopup(t *testing.T) {
	s := screen(t, 80)
	s.Hover.Popup = s.openPopup(hover.Content{Blocks: []hover.Block{para(hover.Span{Text: "a b c"}), {Kind: hover.Rule}, para(hover.Span{Text: "d"})}}, 10, 5)
	p := s.Hover.Popup

	lines := plainLines(s)

	assert.Equal(t, "│"+strings.Repeat("─", p.W-popupFrame)+"│", lines[p.Y+2][p.X:p.X+len("│")*2+len("─")*(p.W-popupFrame)])
}

// TestAnAnswerOfMarkdownReachesTheTerminalOnlyAsText: the control bytes of
// a server, in any kind of span, an OSC 8 hyperlink among them, show as
// stand-ins, dimmed in the style of their span.
func TestAnAnswerOfMarkdownReachesTheTerminalOnlyAsText(t *testing.T) {
	osc8 := "\x1b]8;;https://evil\x07here\x1b]8;;\x07"
	var spans []hover.Span
	for k := range hover.InlineCode + 1 {
		spans = append(spans, hover.Span{Kind: k, Text: osc8 + evil})
	}
	s := screen(t, 80)
	s.Hover.Popup = s.openPopup(hover.Content{Blocks: []hover.Block{
		para(spans...), {Kind: hover.Heading, Spans: spans}, {Kind: hover.Code, Lang: lang.Go, Code: osc8 + evil},
	}}, 10, 5)
	require.True(t, s.Hover.Popup.Open)

	for _, m := range methods {
		s.Cells = m.method
		noControls(t, render(s, &theme{}))
		noControls(t, withoutTheme(render(s, newTheme())))
	}
	assert.Contains(t, strings.Join(plainLines(s), "\n"), "␛]8;;https://evil␇here")
	for _, st := range []style{popupStyle, popupEmphasisStyle, popupStrongStyle, popupLinkStyle, popupDimStyle, popupCodeSpanStyle, popupCodeStyle(syntax.Keyword)} {
		assert.Equal(t, popupStandIn, st.standIn())
	}
}
