package lang_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"seam/internal/domain/lang"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		path, head string
		want       lang.Lang
	}{
		{"main.go", "", lang.Go},
		{"src/lib.rs", "", lang.Rust},
		{"a.js", "", lang.JavaScript},
		{"a.mjs", "", lang.JavaScript},
		{"a.cjs", "", lang.JavaScript},
		{"a.ts", "", lang.TypeScript},
		{"a.mts", "", lang.TypeScript},
		{"a.cts", "", lang.TypeScript},
		{"App.jsx", "", lang.JSX},
		{"App.tsx", "", lang.TSX},
		{"index.html", "", lang.HTML},
		{"INDEX.HTM", "", lang.HTML},
		{"index.php", "", lang.PHP},
		{"a.py", "", lang.Python},
		{"a.pyi", "", lang.Python},
		{"the extension wins over the content.go", "#!/usr/bin/python\n", lang.Go},
		{"bin/tool", "#!/usr/bin/env python3\nprint(1)\n", lang.Python},
		{"bin/tool", "#!/usr/bin/env -S python3 -u\n", lang.Python},
		{"bin/tool", "#!/usr/local/bin/python2.7", lang.Python},
		{"bin/tool", "#! /usr/bin/node\n", lang.JavaScript},
		{"bin/tool", "#!/usr/bin/env php\n<?php\n", lang.PHP},
		{"bin/tool", "<?php\necho 1;\n", lang.PHP},
		{"bin/tool", "#!/bin/sh\n", ""},
		{"bin/tool", "#!/usr/bin/env\n", ""},
		{"bin/tool", "#!\n", ""},
		{"README", "python\n", ""},
		{"Makefile", "", ""},
		{"a.yaml", "", ""},
		{"dir.go/file", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.path+" "+tt.head, func(t *testing.T) {
			assert.Equal(t, tt.want, lang.Defaults().Detect(tt.path, tt.head))
		})
	}
}

func TestFromFence(t *testing.T) {
	tests := map[string]lang.Lang{
		"go": lang.Go, "golang": lang.Go, "Go": lang.Go, " go ": lang.Go,
		"rust": lang.Rust, "rs": lang.Rust,
		"js": lang.JavaScript, "javascript": lang.JavaScript,
		"ts": lang.TypeScript, "typescript": lang.TypeScript,
		"jsx": lang.JSX, "javascriptreact": lang.JSX,
		"tsx": lang.TSX, "typescriptreact": lang.TSX,
		"html": lang.HTML, "php": lang.PHP,
		"python": lang.Python, "py": lang.Python, "python3": lang.Python,
		"": "", "text": "", "yaml": "",
	}
	for name, want := range tests {
		assert.Equal(t, want, lang.Defaults().FromFence(name), name)
	}
}

// TestEveryLanguageIsItsOwnFence: the value of a language is the name a
// fence gives it, so a highlighter can take either.
func TestEveryLanguageIsItsOwnFence(t *testing.T) {
	table := lang.Defaults()
	for _, l := range table.Languages() {
		assert.Equal(t, l.Lang, table.FromFence(string(l.Lang)))
	}
}

func TestDefaultsAreThePlansLanguages(t *testing.T) {
	assert.Equal(t, []lang.Lang{lang.Go, lang.Rust, lang.JavaScript, lang.TypeScript, lang.JSX, lang.TSX, lang.HTML, lang.PHP, lang.Python}, langs(lang.Defaults()))
}

func langs(t lang.Table) []lang.Lang {
	var all []lang.Lang
	for _, l := range t.Languages() {
		all = append(all, l.Lang)
	}
	return all
}
