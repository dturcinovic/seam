package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/textcell"
)

// rendered is what write writes on a line of width columns, without
// colours.
func rendered(width int, write func(*line)) string {
	var b strings.Builder
	ln := newLine(&b, &theme{}, textcell.Codepoints, width)
	write(&ln)
	return b.String()
}

func TestAFileLine(t *testing.T) {
	counted := func(a, d int) change.LineCounts { return change.LineCounts{Added: a, Deleted: d, Known: true} }
	tests := []struct {
		name  string
		file  change.ChangedFile
		width int
		want  string
	}{
		{"counts in columns", change.ChangedFile{Status: change.Modified, NewPath: "x/a.go", Counts: counted(3, 12)},
			24, " │ M a.go      +3   -12 "},
		{"a long name loses its end", change.ChangedFile{Status: change.Added, NewPath: "filelist_test.go", Counts: counted(387, 0)},
			24, " │ A filel…  +387    -0 "},
		{"no counts", change.ChangedFile{Status: change.Untracked, NewPath: "u.txt"}, 24, " │ ? u.txt              "},
		{"too narrow for counts", change.ChangedFile{Status: change.Modified, NewPath: "name.go", Counts: counted(1, 1)},
			14, " │ M name.go  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rendered(tt.width, func(ln *line) { writeFile(ln, tt.file, 6, plainShade) })

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAHeadingLine(t *testing.T) {
	it := listItem{file: -1, dir: "internal/adapter/driving/tui/", count: 12}

	assert.Equal(t, " internal/adapter/driving/tui/      12 ", rendered(39, func(ln *line) { writeHeading(ln, it) }),
		"the count at the right edge")
	assert.Equal(t, " …/driving/tui/ 12 ", rendered(19, func(ln *line) { writeHeading(ln, it) }), "cut at the left")
	assert.Equal(t, " ./ 2 ", rendered(6, func(ln *line) { writeHeading(ln, listItem{file: -1, count: 2}) }))
}

func TestTheTotalsLine(t *testing.T) {
	l := fileList{At: make([]int, 25), CountWidth: 6, Total: change.LineCounts{Added: 1081, Deleted: 75, Known: true}}

	assert.Equal(t, " 25 files · [ ] dir   +1081   -75 ", rendered(34, func(ln *line) { writeTotals(ln, &l) }))
}

func TestTheColumnsFitTheTotals(t *testing.T) {
	fs := files("a.go", "b.go")
	fs[0].Counts = change.LineCounts{Added: 123400, Known: true}
	fs[1].Counts = change.LineCounts{Added: 56, Deleted: 3, Known: true}
	var l fileList

	l.index(fs[1:], textcell.Codepoints)
	assert.Equal(t, minCountWidth, l.CountWidth)
	l.index(fs, textcell.Codepoints)
	assert.Equal(t, len(" +123456"), l.CountWidth)
}

func TestTotalsTooNarrowForTheirCounts(t *testing.T) {
	l := fileList{At: make([]int, 3), CountWidth: 5, Total: change.LineCounts{Added: 12, Deleted: 345, Known: true}}

	assert.Equal(t, " 3 files… ", rendered(10, func(ln *line) { writeTotals(ln, &l) }), "no cut number")
}

func TestTheListIsWideEnoughForItsTotals(t *testing.T) {
	s := newState(change.Changeset{Files: manyFiles(25)}, splitSettings())

	assert.Equal(t, totalsWidth(25, s.List.CountWidth, s.Cells), s.List.Longest)
}

func TestAStandInInAPathIsDimmed(t *testing.T) {
	for r := range listShades {
		assert.Equal(t, listStyle(listDimPart, r), listStyle(listNamePart, r).standIn())
	}
}
