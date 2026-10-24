package tui

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"seam/internal/domain/change"
	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/ui"
	"seam/internal/testsupport"
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
	hunks := shown(t, files("cmd/main.go", "cmd/z.go"), 100, 16, filediff.Snapshot{Diff: code})
	full := shown(t, files("cmd/main.go", "cmd/z.go"), 100, 28, filediff.Snapshot{Diff: code})
	full.toggleMode()
	// Longer than the screen: the thumb shows, and the marks share cells.
	scrolled := shown(t, files("cmd/main.go", "cmd/z.go"), 100, 8, filediff.Snapshot{Diff: code})
	scrolled.toggleMode()
	scrolled.scrollTo(12)
	// Wide enough for the file list to open by itself, once with the focus.
	list := shown(t, listFiles(), 140, 9, filediff.Snapshot{Diff: code})
	focused := shown(t, listFiles(), 140, 9, filediff.Snapshot{Diff: code})
	focused.toggleList()
	focused.moveCursor(2)
	// Narrower than split_min_width: unified by itself.
	unifiedHunks := shownWith(t, ui.DefaultSettings(), files("cmd/main.go"), 100, 16, filediff.Snapshot{Diff: code})
	unifiedFull := shownWith(t, ui.DefaultSettings(), files("cmd/main.go"), 100, 30, filediff.Snapshot{Diff: code})
	unifiedFull.toggleMode()

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
	return append([]fixture{
		{"hunks", hunks}, {"full", full}, {"scrolled", scrolled}, {"list", list}, {"list-focused", focused},
		{"unified-hunks", unifiedHunks}, {"unified-full", unifiedFull},
	}, unicode...)
}

// TestGolden renders each fixture with the real theme. cat shows a golden
// file in colour.
func TestGolden(t *testing.T) {
	for _, f := range fixtures(t) {
		t.Run(f.name, func(t *testing.T) {
			testsupport.Golden(t, filepath.Join("testdata", f.name+".golden"), render(f.state, newTheme(ui.Dark()))+"\n", *update)
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
	unified := strings.Join(byName["unified-hunks"], "\n")
	assert.Contains(t, unified, "hunks unified")
	assert.Contains(t, unified, ` 17 17   func main() {`)
	assert.Contains(t, unified, ` 18    -     fmt.Println("a finding, an`)
	assert.Contains(t, unified, `    18 +     fmt.Println("a finding and`)
	assert.Contains(t, unified, ` 19    -     gone()`)
	assert.Contains(t, strings.Join(byName["unified-full"], "\n"), "  1  1   package main")
	assert.Contains(t, byName["unicode-codepoints-scroll0"][4], "a   b")
	assert.Contains(t, byName["unicode-codepoints-scroll0"][5], "日  b")
	assert.Contains(t, byName["unicode-graphemes-scroll0"][6], "👨\u200d👩\u200d👧  b")
	assert.Contains(t, byName["unicode-graphemes-scroll1"][3], "  本語")
}

// listFiles has a path too long for the list, counts of every width, an
// untracked file and a binary one, which have none.
func listFiles() []change.ChangedFile {
	known := func(a, d int) change.LineCounts { return change.LineCounts{Added: a, Deleted: d, Known: true} }
	return []change.ChangedFile{
		{Status: change.Modified, OldPath: "cmd/main.go", NewPath: "cmd/main.go", Counts: known(2, 3)},
		{Status: change.Added, NewPath: "internal/adapter/driving/tui/filelist.go", Counts: known(120, 0)},
		{Status: change.Deleted, OldPath: "old.go", Counts: known(0, 7)},
		{Status: change.Modified, OldPath: "logo.png", NewPath: "logo.png"},
		{Status: change.Untracked, NewPath: "notes.txt"},
	}
}
