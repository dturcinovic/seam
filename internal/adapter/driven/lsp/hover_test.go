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

func TestALineBreakFarBackDoesNotCostTheCut(t *testing.T) {
	text := "func F()\n" + strings.Repeat("x", hover.MaxText)

	parts, cut := cutParts([]part{{format: markdownPart, text: text}}, hover.MaxText)

	assert.True(t, cut)
	assert.Equal(t, text[:hover.MaxText], parts[0].text)
}
