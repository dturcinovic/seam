package tui

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
)

var update = flag.Bool("update", false, "rewrite the golden files from the current output")

type fixture struct {
	name  string
	state *state
}

// fixtures are the screens the golden files hold. The unicode one has a
// combining é, a ZWJ sequence, CJK, tabs after text of different widths,
// and is scrolled into the middle of a wide character.
func fixtures(t *testing.T) []fixture {
	t.Helper()
	old := "package main\n\nimport \"fmt\"\n\n" + numbered(12) +
		"func main() {\n\tfmt.Println(\"a finding, an investigation note, and a report the\")\n\tgone()\n}\n" +
		"// " + strings.Repeat("long ", 30) + "\n"
	new := "package main\n\nimport \"fmt\"\n\n" + numbered(12) +
		"func main() {\n\tfmt.Println(\"a finding and an investigation note.\")\n}\n" +
		"// " + strings.Repeat("long ", 30) + "end\n" + "func added() {}\n"
	code := diff.Compute(t.Context(), diff.Split(old), diff.Split(new))
	hunks := shown(t, files("cmd/main.go", "b.go"), 100, 16, filediff.Snapshot{Diff: code})
	full := shown(t, files("cmd/main.go", "b.go"), 100, 28, filediff.Snapshot{Diff: code})
	full.toggleMode()

	text := diff.Compute(t.Context(),
		diff.Split("cafe\nfamily 👨\u200d👩\u200d👧 x\n日本語のテキスト\na\tb\n日\tb\n👨\u200d👩\u200d👧\tb\n"),
		diff.Split("cafe\u0301\nfamily 👨\u200d👩\u200d👧 y\n日本語のテキスト\na\tb\n日\tb\n👨\u200d👩\u200d👧\tb\n"))
	var unicode []fixture
	for _, m := range methods {
		for _, scroll := range []int{0, 1} {
			s := shown(t, files("名前.txt"), 60, 9, filediff.Snapshot{Diff: text})
			s.toggleMode()
			s.Cells, s.HScroll = m.method, scroll
			unicode = append(unicode, fixture{fmt.Sprintf("unicode-%s-scroll%d", m.name, scroll), s})
		}
	}
	return append([]fixture{{"hunks", hunks}, {"full", full}}, unicode...)
}

// TestGolden renders each fixture with the real theme. cat shows a golden
// file in colour.
func TestGolden(t *testing.T) {
	for _, f := range fixtures(t) {
		t.Run(f.name, func(t *testing.T) {
			got := render(f.state, newTheme()) + "\n"
			path := filepath.Join("testdata", f.name+".golden")
			if *update {
				require.NoError(t, os.MkdirAll("testdata", 0o755))
				require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
			}
			want, err := os.ReadFile(path)
			require.NoError(t, err, "run with -update to create it")
			assert.Equal(t, string(want), got)
		})
	}
}

func TestTheGoldenFilesShowTheirCases(t *testing.T) {
	byName := map[string][]string{}
	for _, f := range fixtures(t) {
		byName[f.name] = plainLines(f.state)
	}

	hunks := strings.Join(byName["hunks"], "\n")
	assert.Contains(t, hunks, "⋯ 14 unchanged lines")
	assert.Contains(t, hunks, `18 ~     fmt.Println("a finding, an`)
	assert.Contains(t, hunks, "19 -     gone()")
	assert.Contains(t, strings.Join(byName["full"], "\n"), " 1   package main")
	assert.Contains(t, byName["unicode-codepoints-scroll0"][4], "a   b")
	assert.Contains(t, byName["unicode-codepoints-scroll0"][5], "日  b")
	assert.Contains(t, byName["unicode-graphemes-scroll0"][6], "👨\u200d👩\u200d👧  b")
	assert.Contains(t, byName["unicode-graphemes-scroll1"][3], "  本語")
}
