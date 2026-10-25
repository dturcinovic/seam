package lsp

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/lang"
	"github.com/dturcinovic/seam/internal/testsupport"
)

var update = flag.Bool("update", false, "rewrite the golden files from the current output")

func paragraph(text string) hover.Content {
	return hover.Content{Blocks: []hover.Block{{Kind: hover.Paragraph, Spans: []hover.Span{{Text: text}}}}}
}

// shownContent writes c as text with a mark for each kind: code fenced
// with its language, rules as ---, headings after #, and within text
// *emphasis*, **strong**, `code`, [link text] and <url>.
func shownContent(c hover.Content) string {
	var b strings.Builder
	for i, bl := range c.Blocks {
		if i > 0 {
			b.WriteString("\n")
		}
		switch bl.Kind {
		case hover.Code:
			b.WriteString("```" + string(bl.Lang) + "\n" + bl.Code + "\n```\n")
		case hover.Rule:
			b.WriteString("---\n")
		case hover.Heading:
			b.WriteString("# " + shownSpans(bl.Spans) + "\n")
		case hover.Paragraph:
			b.WriteString(shownSpans(bl.Spans) + "\n")
		}
	}
	if c.Cut {
		b.WriteString("(cut)\n")
	}
	return b.String()
}

func shownSpans(spans []hover.Span) string {
	marks := map[hover.SpanKind][2]string{
		hover.Emphasis: {"*", "*"}, hover.Strong: {"**", "**"}, hover.InlineCode: {"`", "`"},
		hover.Link: {"[", "]"}, hover.URL: {"<", ">"},
	}
	var b strings.Builder
	for _, s := range spans {
		m := marks[s.Kind]
		b.WriteString(m[0] + s.Text + m[1])
	}
	return b.String()
}

type fixture struct {
	Server    string         `json:"server"`
	Version   string         `json:"version"`
	File      string         `json:"file"`
	Line      int            `json:"line"`
	Character int            `json:"character"`
	Word      string         `json:"word"`
	Result    jsontext.Value `json:"result"`
}

// TestRealAnswersBecomeBlocks reads the answers of every server preset,
// as each gave them at its position in testdata/hover, and compares their
// blocks with the golden file beside them.
func TestRealAnswersBecomeBlocks(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "hover", "*.json"))
	require.NoError(t, err)
	servers := map[string]bool{}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			var f fixture
			require.NoError(t, json.Unmarshal(raw, &f))
			servers[f.Server] = true

			c, err := hoverContent(f.Result, lang.Defaults())

			require.NoError(t, err)
			require.NotEmpty(t, c.Blocks, "%s %s at %s:%d:%d", f.Server, f.Version, f.File, f.Line, f.Character)
			testsupport.Golden(t, strings.TrimSuffix(path, ".json")+".golden", shownContent(c), *update)
		})
	}
	for _, s := range lang.Defaults().Servers() {
		assert.True(t, servers[s.Name], "no answer of %s", s.Name)
	}
}

func TestContentsInEveryForm(t *testing.T) {
	tests := []struct {
		name, result, want string
	}{
		{"none", `null`, ""},
		{"no contents", `{"contents": []}`, ""},
		{"blank", `{"contents": {"kind": "markdown", "value": " \n\n"}}`, ""},
		{"markup", `{"contents": {"kind": "markdown", "value": "*a*"}}`, "*a*\n"},
		{"plain text keeps its marks and lines", `{"contents": {"kind": "plaintext", "value": "*a*\nb\n\nc"}}`, "*a*\nb\n\nc\n"},
		{"marked string", `{"contents": "a"}`, "a\n"},
		{"marked code", `{"contents": {"language": "golang", "value": "func f()\r\n\tx\r\n"}}`, "```go\nfunc f()\n\tx\n```\n"},
		{"list", `{"contents": [{"language": "python", "value": "x"}, "", "doc"]}`, "```python\nx\n```\n\ndoc\n"},
		{"carriage returns", `{"contents": {"kind": "markdown", "value": "a\r\nb\r\n\r\n` + "```go\\r\\nx\\r\\n```" + `"}}`, "a b\n\n```go\nx\n```\n"},
		{"plain carriage returns", `{"contents": {"kind": "plaintext", "value": "a\r\n\r\nb"}}`, "a\n\nb\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := hoverContent(jsontext.Value(tt.result), lang.Defaults())

			require.NoError(t, err)
			assert.Equal(t, tt.want, shownContent(c))
		})
	}
	_, err := hoverContent(jsontext.Value(`{"contents": 1`), lang.Defaults())
	assert.Error(t, err)
}

func TestMarkdownBlocks(t *testing.T) {
	tests := []struct {
		name, md, want string
	}{
		{"fence without language", "```\nx\n```", "```\nx\n```\n"},
		{"fence of an unknown language", "```cobol\nx\n```", "```\nx\n```\n"},
		{"code span at the start of a line", "```go func```\nrest", "`go func` rest\n"},
		{"tilde fence keeps backticks", "~~~go\na ```\n~~~", "```go\na ```\n```\n"},
		{"unclosed fence runs to the end", "```go\nx\n\ny", "```go\nx\n\ny\n```\n"},
		{"fence keeps its blank lines", "```go\na\n\n\tb\n```", "```go\na\n\n\tb\n```\n"},
		{"indented fence loses its indent", "  ```go\n  a\n    b\n  ```", "```go\na\n  b\n```\n"},
		{"indented code after a blank line", "For example:\n\n\tq := 1\n\t  r\n\nmore", "For example:\n\n```\nq := 1\n  r\n```\n\nmore\n"},
		{"indented line continues a paragraph", "a\n    b", "a b\n"},
		{"indented line continues a list", "- a\n\n    b", "• a\n\nb\n"},
		{"rules", "a\n\n---\n***\n\n_ _ _\nb", "a\n\n---\n\nb\n"},
		{"rules at the ends go", "---\na\n---", "a\n"},
		{"rule right after a fence", "```py\nx\n```\n---\nDoc", "```python\nx\n```\n\n---\n\nDoc\n"},
		{"headings", "# A #\n### B\n#C\n## C#\n# #", "# A\n\n# B\n\n#C\n\n# C#\n"},
		{"soft and hard breaks", "a\nb  \nc\\\nd", "a b\nc\nd\n"},
		{"list items", "It holds:\n\n  - X\n  - Y\n* Z\n1. one\n2) two", "It holds:\n\n• X\n• Y\n• Z\n1. one\n2) two\n"},
		{"paragraph text right after a fence", "```ts\nf()\n```\nAdds.", "```typescript\nf()\n```\n\nAdds.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shownContent(hover.Content{Blocks: tidy(markdownBlocks(tt.md, lang.Defaults()))})

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestInlineMarkdown(t *testing.T) {
	defs := "\n\n[`std::fmt`]: https://doc.rust-lang.org/fmt\n[Ref]: <https://example.com/r> \"title\""
	tests := []struct {
		name, md, want string
	}{
		{"escapes", `\*sum\* \` + "`x\\` a\\b", "*sum* `x` a\\b"},
		{"code", "the `Vec<T>` and ``a ` b`` and ` `` `", "the `Vec<T>` and `a ` b` and ````"},
		{"unclosed code", "a `b", "a `b"},
		{"emphasis", "*@param* `a` — the **sum**, _x_ and __add__", "*@param* `a` — the **sum**, *x* and **add**"},
		{"no emphasis within words or before blanks", "snake_case_name a * b * c 2*3", "snake_case_name a * b * c 2*3"},
		{"bold italics", "***both*** and ___too___", "**both** and **too**"},
		{"code hides delimiters", "Returns *T, see `a*b` here.", "Returns *T, see `a*b` here."},
		{"emphasis over a soft break", "*Note that\nthe end*", "*Note that the end*"},
		{"link", "see [docs](https://example.com) now", "see [docs] <https://example.com> now"},
		{"link with a title", `[a](https://x.org "T")`, "[a] <https://x.org>"},
		{"link with code", "[`fmt.Println` on pkg.go.dev](https://pkg.go.dev/fmt#Println)", "`fmt.Println`[ on pkg.go.dev] <https://pkg.go.dev/fmt#Println>"},
		{"link to itself", "[https://go.dev](https://go.dev)", "<https://go.dev>"},
		{"link elsewhere shows no target", "[`vec!`](`vec!`)", "`vec!`"},
		{"file link", "[B](file:///b.go#1,2)", "[B] <file:///b.go#1,2>"},
		{"nested brackets", "[`Box<[T]>`](https://b.org)", "`Box<[T]>` <https://b.org>"},
		{"reference", "See [`std::fmt`] and [this][ref] and [Ref][]", "See `std::fmt` <https://doc.rust-lang.org/fmt> and [this] <https://example.com/r> and [Ref] <https://example.com/r>"},
		{"unknown reference stays text", "Use [`get`] or [x]", "Use [`get`] or [x]"},
		{"image shows its text only", "![Baseline icon](data:image/svg+xml;base64,PHN2Zw==) ok", "Baseline icon ok"},
		{"autolink", "_@link_ <https://php.net/x> <b>", "*@link* <https://php.net/x> <b>"},
		{"unclosed bracket", "a [b c", "a [b c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := markdownBlocks(tt.md+defs, lang.Defaults())

			require.Len(t, blocks, 1)
			assert.Equal(t, tt.want, shownSpans(blocks[0].Spans))
		})
	}
}

func TestALongAnswerIsCutAtALineBreak(t *testing.T) {
	line := strings.Repeat("x", 99) + "\n"
	long := strings.Repeat(line, hover.MaxText/len(line)+10)
	result, err := json.Marshal(map[string]any{"contents": []any{map[string]string{"language": "go", "value": "func f()"}, long, "after"}})
	require.NoError(t, err)

	c, err := hoverContent(result, lang.Defaults())

	require.NoError(t, err)
	assert.True(t, c.Cut)
	text := shownSpans(c.Blocks[1].Spans)
	assert.LessOrEqual(t, len("func f()")+len(text), hover.MaxText)
	assert.Equal(t, strings.TrimSuffix(strings.ReplaceAll(long[:len(text)+1], "\n", " "), " "), text, "whole lines only")
	assert.Len(t, c.Blocks, 2, "nothing after the cut")
}

func TestALongLineIsCutAtACharacter(t *testing.T) {
	parts, cut := cutParts([]part{{format: markdownPart, text: "ab" + strings.Repeat("日", 10)}}, 6)

	assert.True(t, cut)
	assert.Equal(t, "ab日", parts[0].text)
}

// TestControlsInAnAnswerStayInItsText: the blocks hold the answer's bytes
// as sent; the viewer replaces them as it does everywhere, so an escape
// sequence of the server never reaches the terminal as one.
func TestControlsInAnAnswerStayInItsText(t *testing.T) {
	md := "a\x1b]8;;https://evil\x07link\x1b]8;;\x07 \x1b[2J"

	blocks := markdownBlocks(md, lang.Defaults())

	assert.Equal(t, md, shownSpans(blocks[0].Spans))
}

func TestALineBreakFarBackDoesNotCostTheCut(t *testing.T) {
	text := "func F()\n" + strings.Repeat("x", hover.MaxText)

	parts, cut := cutParts([]part{{format: markdownPart, text: text}}, hover.MaxText)

	assert.True(t, cut)
	assert.Equal(t, text[:hover.MaxText], parts[0].text)
}
