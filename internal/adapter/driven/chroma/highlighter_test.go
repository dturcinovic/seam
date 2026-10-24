package chroma_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/zap"

	"seam/internal/adapter/driven/chroma"
	"seam/internal/domain/diff"
	"seam/internal/domain/lang"
	"seam/internal/domain/syntax"
	"seam/internal/testsupport"
)

var update = flag.Bool("update", false, "rewrite the golden files from the current output")

func TestMain(m *testing.M) {
	// chroma gives each regex match 250 ms; regexp2 keeps a clock goroutine
	// for such deadlines, which ends by itself a second after the last.
	goleak.VerifyTestMain(m, goleak.IgnoreAnyFunction("github.com/dlclark/regexp2/v2.runClock"))
}

func newHighlighter(t testing.TB) *chroma.Highlighter {
	t.Helper()
	h, err := chroma.New(zap.NewNop())
	require.NoError(t, err)
	return h
}

func highlight(t *testing.T, l lang.Lang, text string) syntax.Tokens {
	t.Helper()
	tok, _ := newHighlighter(t).Highlight(t.Context(), l, syntax.Lexable(text))
	return tok
}

func fixture(t *testing.T, name string) (lang.Lang, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	l := lang.Defaults().Detect(name, string(data))
	require.NotEmpty(t, l, name)
	return l, string(data)
}

// kindNames name the kinds in the golden files and messages.
var kindNames = [syntax.Kinds]string{
	"plain", "keyword", "type", "function", "builtin", "constant", "string", "escape",
	"number", "comment", "meta", "operator", "punctuation", "tag", "attribute", "variable",
}

// kindsOf returns the kind of each byte of line i, plain past its runs.
func kindsOf(tok syntax.Tokens, i int, line string) []syntax.Kind {
	kinds := make([]syntax.Kind, len(line))
	at := 0
	for _, r := range tok.Line(i) {
		for ; at < int(r.End) && at < len(line); at++ {
			kinds[at] = r.Kind
		}
	}
	return kinds
}

// kindOf returns the kind of needle in line i of text, failing unless all
// its bytes have one kind.
func kindOf(t *testing.T, tok syntax.Tokens, text string, i int, needle string) syntax.Kind {
	t.Helper()
	line := diff.Split(text).Lines[i]
	at := strings.Index(line, needle)
	require.GreaterOrEqual(t, at, 0, "%q not in line %d: %q", needle, i, line)
	kinds := kindsOf(tok, i, line)[at : at+len(needle)]
	for _, k := range kinds[1:] {
		require.Equal(t, kindNames[kinds[0]], kindNames[k], "%q in line %d has more than one kind", needle, i)
	}
	return kinds[0]
}

type expect struct {
	line   int
	needle string
	kind   syntax.Kind
}

// TestEveryLanguageGetsItsKinds loads the fixtures of all languages of the
// plan and checks the kinds that matter for each, among them the tags and
// embedded expressions of JSX and TSX and the HTML around PHP.
func TestEveryLanguageGetsItsKinds(t *testing.T) {
	tests := map[string][]expect{
		"sample.go": {
			{0, "// Package sample is a fixture.", syntax.Comment},
			{1, "package", syntax.Keyword},
			{3, `"fmt"`, syntax.String},
			{5, "/* A block comment", syntax.Comment},
			{6, "over three lines", syntax.Comment},
			{7, "ends here. */", syntax.Comment},
			{9, "type", syntax.Keyword},
			{9, "int64", syntax.Type},
			{11, "Hello", syntax.Function},
			{11, "string", syntax.Type},
			{12, "const", syntax.Keyword},
			{12, "42", syntax.Number},
			{13, `\n`, syntax.String},
			{13, "len", syntax.Builtin},
			{13, "nil", syntax.Constant},
			{14, "return", syntax.Keyword},
		},
		"sample.rs": {
			{0, "// A fixture.", syntax.Comment},
			{1, "use", syntax.Keyword},
			{3, "#[derive(Debug)]", syntax.Meta},
			{4, "struct", syntax.Keyword},
			{4, "i32", syntax.Type},
			{6, "fn", syntax.Keyword},
			{6, "main", syntax.Function},
			{7, `"hi"`, syntax.String},
			{8, "println!", syntax.Function},
			{8, "7", syntax.Number},
		},
		"sample.js": {
			{0, "// A fixture.", syntax.Comment},
			{1, "import", syntax.Keyword},
			{1, `"node:fs"`, syntax.String},
			{3, "function", syntax.Keyword},
			{4, "42", syntax.Number},
			{5, "`hi ", syntax.String},
		},
		"sample.ts": {
			{1, "interface", syntax.Keyword},
			{1, "number", syntax.Type},
			{3, "function", syntax.Keyword},
			{4, "42", syntax.Number},
			{5, `"hi"`, syntax.String},
		},
		"sample.jsx": {
			{1, "function", syntax.Keyword},
			{2, "div", syntax.Tag},
			{2, "className", syntax.Attribute},
			{2, `"greeting"`, syntax.String},
			{2, "()", syntax.Punctuation}, // the embedded expression is code
		},
		"sample.tsx": {
			{1, "type", syntax.Keyword},
			{1, "string", syntax.Type},
			{4, "section", syntax.Tag},
			{4, "id", syntax.Attribute},
			{4, `"main"`, syntax.String},
			{5, "h1", syntax.Tag},
			{5, "className", syntax.Attribute},
			{5, "3", syntax.Number},
			{5, `"short"`, syntax.String},
		},
		"sample.html": {
			{0, "<!DOCTYPE html>", syntax.Meta},
			{1, "html", syntax.Tag},
			{1, "lang", syntax.Attribute},
			{1, `"en"`, syntax.String},
			{2, "<!-- a comment -->", syntax.Comment},
			{4, "&amp;", syntax.Escape},
			{5, "script", syntax.Tag},
			{5, "const", syntax.Keyword},
			{5, "42", syntax.Number},
		},
		"sample.php": {
			{0, "html", syntax.Tag},
			{1, "class", syntax.Attribute},
			{1, `"page"`, syntax.String},
			{2, "<?php", syntax.Meta},
			{3, "// A fixture.", syntax.Comment},
			{4, "function", syntax.Keyword},
			{4, "$name", syntax.Variable},
			{5, `"hi "`, syntax.String},
			{5, "42", syntax.Number},
			{7, "?>", syntax.Meta},
			{8, "p", syntax.Tag},
			{8, "<?", syntax.Meta},
			{8, `"you"`, syntax.String},
			{9, "body", syntax.Tag},
		},
		"sample.py": {
			{0, "# A fixture.", syntax.Comment},
			{1, "import", syntax.Keyword},
			{3, "@dataclass", syntax.Meta},
			{4, "class", syntax.Keyword},
			{4, "Point", syntax.Type},
			{5, `"""A point."""`, syntax.String},
			{6, "int", syntax.Builtin},
			{6, "42", syntax.Number},
			{8, "def", syntax.Keyword},
			{8, "hello", syntax.Function},
			{9, "len", syntax.Builtin},
		},
	}
	for name, checks := range tests {
		t.Run(name, func(t *testing.T) {
			l, text := fixture(t, name)
			tok := highlight(t, l, text)

			require.Equal(t, len(diff.Split(text).Lines), tok.Lines(), "every line covered")
			for _, c := range checks {
				assert.Equal(t, kindNames[c.kind], kindNames[kindOf(t, tok, text, c.line, c.needle)], "line %d %q", c.line, c.needle)
			}
			testsupport.Golden(t, filepath.Join("testdata", name+".golden"), annotate(tok, text), *update)
		})
	}
}

// annotate writes each line's runs as kind:text, so a golden file shows
// every kind the lexer found.
func annotate(tok syntax.Tokens, text string) string {
	var b strings.Builder
	for i, line := range diff.Split(text).Lines {
		content := diff.Content(line)
		at := 0
		fmt.Fprintf(&b, "%3d", i+1)
		for _, r := range tok.Line(i) {
			end := min(int(r.End), len(content))
			if end > at && r.Kind != syntax.Plain {
				fmt.Fprintf(&b, " %s:%q", kindNames[r.Kind], content[at:end])
			}
			at = int(r.End)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
