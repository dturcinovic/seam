package chroma_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dturcinovic/seam/internal/domain/lang"
	"github.com/dturcinovic/seam/internal/domain/syntax"
)

// BenchmarkGoFile tokenises a Go file of 5000 lines; the plan wants it
// under 200 ms.
func BenchmarkGoFile(b *testing.B) {
	var src strings.Builder
	src.WriteString("package sample\n\nimport \"fmt\"\n\n")
	for i, lines := 0, 4; lines < 5000; i, lines = i+1, lines+7 {
		fmt.Fprintf(&src, "// F%d greets.\nfunc F%d(name string) (int, error) {\n\tn := len(name) + %d\n\tfmt.Println(\"hi\", name, n)\n\treturn n, nil\n}\n\n", i, i, i)
	}
	text := src.String()
	h := newHighlighter(b)
	for b.Loop() {
		h.Highlight(context.Background(), lang.Go, text)
	}
}

// worstShapes are the inputs that keep a lexer longest in one token, or in
// one delegated or embedded part, per language: 8 MiB in lines just under
// syntax.MaxLine, each a comment, a string, or many short tokens, or one
// comment, raw string or script around all lines.
func worstShapes(l lang.Lang) map[string]string {
	body := strings.Repeat("x", syntax.MaxLine-64)
	quote := `"`
	comment, ok := map[lang.Lang][2]string{
		lang.Python: {"# ", ""},
		lang.HTML:   {"<!-- ", " -->"},
		lang.PHP:    {"<?php /* ", " */ ?>"},
	}[l]
	if !ok {
		comment = [2]string{"/* ", " */"}
	}
	open, close := comment[0], comment[1]
	shapes := map[string]string{
		"comment": open + body + close,
		"string":  "x = " + quote + body + quote,
		"tokens":  strings.Repeat("a+", len(body)/2),
	}
	if l == lang.PHP {
		shapes["string"] = "<?php $x = " + quote + body + quote + "; ?>"
		shapes["tokens"] = "<?php " + strings.Repeat("a+", len(body)/2) + " ?>"
	}
	for name, line := range shapes {
		shapes[name] = strings.Repeat(line+"\n", (8<<20)/(len(line)+1))
	}
	lines := strings.Repeat(body+"\n", (8<<20)/(len(body)+1)-1)
	shapes["around"] = open + "\n" + lines + close + "\n"
	if l == lang.Go {
		shapes["raw string"] = "var x = `\n" + lines + "`\n"
	}
	if l == lang.HTML {
		shapes["script"] = "<script>\n" + strings.ReplaceAll(lines, "x", "a+") + "</script>\n"
	}
	return shapes
}

// BenchmarkBudgetOverrun measures how long each lexer runs on past a
// budget of 500 ms with its worst input: the soft bound the plan asks to be
// under 250 ms.
func BenchmarkBudgetOverrun(b *testing.B) {
	const budget = 500 * time.Millisecond
	h := newHighlighter(b)
	for _, language := range lang.Defaults().Languages() {
		l := language.Lang
		for shape, text := range worstShapes(l) {
			b.Run(string(l)+"/"+shape, func(b *testing.B) {
				var longest time.Duration
				for b.Loop() {
					ctx, cancel := context.WithTimeout(context.Background(), budget)
					begin := time.Now()
					h.Highlight(ctx, l, text)
					longest = max(longest, time.Since(begin))
					cancel()
				}
				// Negative when the input took less than the budget.
				b.ReportMetric(float64((longest - budget).Milliseconds()), "overrun-ms")
			})
		}
	}
}
