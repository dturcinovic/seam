//go:build integration

package lsp

import (
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/lang"
	"github.com/dturcinovic/seam/internal/testsupport"
)

// TestServers asks each default server for a hover in a small project of
// its own. A server that is not installed is skipped, not passed.
func TestServers(t *testing.T) {
	tests := []struct {
		name     string
		needs    []string
		files    map[string]string
		path     string
		lang     lang.Lang
		at, word string
		want     string
	}{
		{
			name: "rust", needs: []string{"rust-analyzer", "cargo"},
			files: map[string]string{
				"Cargo.toml": "[package]\nname = \"demo\"\nversion = \"0.1.0\"\nedition = \"2021\"\n",
				"src/lib.rs": "/// Greets.\npub fn greet(name: &str) -> String {\n    format!(\"hi {name}\")\n}\n\npub fn twice() -> String {\n    greet(\"a\")\n}\n",
			},
			path: "src/lib.rs", lang: lang.Rust, at: "    greet(\"a\")", word: "greet",
			want: "pub fn greet(name: &str) -> String",
		},
		{
			name: "typescript with a local import", needs: []string{"tsc"},
			files: map[string]string{
				"tsconfig.json": "{\"compilerOptions\":{\"strict\":true}}\n",
				"greet.ts":      "export function greet(name: string): string {\n  return `hi ${name}`;\n}\n",
				"main.ts":       "import { greet } from \"./greet\";\nconsole.log(greet(\"a\"));\n",
			},
			path: "main.ts", lang: lang.TypeScript, at: "console.log(greet(\"a\"));", word: "greet",
			want: "function greet(name: string): string",
		},
		{
			name: "javascript", needs: []string{"tsc"},
			files: map[string]string{"package.json": "{}\n", "main.js": "function twice(n) {\n  return n * 2;\n}\ntwice(2);\n"},
			path:  "main.js", lang: lang.JavaScript, at: "twice(2);", word: "twice",
			want: "function twice(n: any): number",
		},
		{
			name: "tsx", needs: []string{"tsc"},
			files: map[string]string{
				"tsconfig.json": "{\"compilerOptions\":{\"jsx\":\"preserve\",\"strict\":true}}\n",
				"App.tsx":       "function Title(props: { text: string }) {\n  return <h1>{props.text}</h1>;\n}\nexport const app = <Title text=\"a\" />;\n",
			},
			path: "App.tsx", lang: lang.TSX, at: "export const app = <Title text=\"a\" />;", word: "Title",
			want: "function Title(props: {",
		},
		{
			name: "jsx", needs: []string{"tsc"},
			files: map[string]string{"package.json": "{}\n", "App.jsx": "function Title() {\n  return <h1>a</h1>;\n}\nexport const app = <Title />;\n"},
			path:  "App.jsx", lang: lang.JSX, at: "export const app = <Title />;", word: "Title",
			// Without React types the return type is the server's guess.
			want: "function Title()",
		},
		{
			name: "html", needs: []string{"vscode-html-language-server"},
			files: map[string]string{"index.html": "<!doctype html>\n<html>\n<body>\n<div>a</div>\n</body>\n</html>\n"},
			path:  "index.html", lang: lang.HTML, at: "<div>a</div>", word: "div",
			want: "div",
		},
		{
			name: "php", needs: []string{"intelephense"},
			files: map[string]string{"composer.json": "{}\n", "index.php": "<?php\nfunction greet(string $name): string { return \"hi $name\"; }\necho greet('a');\n"},
			path:  "index.php", lang: lang.PHP, at: "echo greet('a');", word: "greet",
			want: "function greet(string $name): string",
		},
		{
			name: "python with a local import", needs: []string{"pyright-langserver"},
			files: map[string]string{
				"pyproject.toml": "[project]\nname = \"demo\"\nversion = \"0.1.0\"\n",
				"greet.py":       "def greet(name: str) -> str:\n    return f\"hi {name}\"\n",
				"main.py":        "from greet import greet\n\nprint(greet(\"a\"))\n",
			},
			path: "main.py", lang: lang.Python, at: "print(greet(\"a\"))", word: "greet",
			want: "def greet(name: str) -> str",
		},
	}
	native := nativeTsc(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, program := range tt.needs {
				if _, err := exec.LookPath(program); err != nil {
					t.Skipf("%s is not installed", program)
				}
			}
			if slices.Contains(tt.needs, "tsc") && !native {
				t.Skip("tsc is older than TypeScript 7, which has no --lsp")
			}
			pool := realPool(t, lang.Defaults(), tt.files)
			text := tt.files[tt.path]
			_, ok := pool.Sync(hover.Document{Path: tt.path, Lang: tt.lang, Version: 1, Text: text})
			require.True(t, ok)
			testsupport.Collect(t, pool, hasStatus(hover.Ready))

			line := strings.Count(text[:strings.Index(text, tt.at)], "\n")
			col := strings.Index(tt.at, tt.word)
			got := hoverUntil(t, pool, hover.Position{Path: tt.path, Version: 1, Line: line, Byte: col}, 90*time.Second)

			assert.Contains(t, got, tt.want)
			stopPool(t, pool)
		})
	}
}

// nativeTsc reports whether tsc is TypeScript 7 or later, the native port
// that serves LSP.
func nativeTsc(t *testing.T) bool {
	out, err := exec.CommandContext(t.Context(), "tsc", "--version").Output()
	if err != nil {
		return false
	}
	major, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimSpace(string(out)), "Version "), ".")
	n, err := strconv.Atoi(major)
	return err == nil && n >= 7
}
