package note_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/note"
)

// file is text split into lines, as tests write it.
func file(text string) []string { return strings.Split(strings.TrimPrefix(text, "\n"), "\n") }

func anchor(t *testing.T, lines []string, start, end int) note.Anchor {
	t.Helper()
	a, err := note.NewAnchor("f.go", note.Origin{Kind: change.KindWorktree}, change.ObjectID{}, lines, start, end)
	require.NoError(t, err)
	return a
}

const twoReturns = `
func a() error {
	x := 1
	use(x)
	return nil
}

func b() error {
	y := 2
	use(y)
	return nil
}`

func TestLocateFindsTheNoteAgain(t *testing.T) {
	twice := "s\ns\ns\nA\nB\ne\ne\ne\n"
	tests := map[string]struct {
		before     string
		start, end int
		after      string
		want       note.Placement
	}{
		"unchanged": {twoReturns, 4, 4, twoReturns, note.Placement{Where: note.Exact, Start: 4, End: 4}},
		"lines inserted above": {twoReturns, 4, 4, "// a\n// b\n// c\n" + twoReturns[1:],
			note.Placement{Where: note.Exact, Start: 7, End: 7}},
		"the same line elsewhere does not matter": {twoReturns, 10, 10, twoReturns,
			note.Placement{Where: note.Exact, Start: 10, End: 10}},
		"deleted, the same line elsewhere with other context": {twoReturns, 4, 4, `
func b() error {
	y := 2
	use(y)
	return nil
}`, note.Placement{Where: note.Orphaned}},
		"deleted, half the context elsewhere is likely": {`
func a() error {
	x := 1
	use(x)
	return nil
}
// one
// two
func b() error {
	x := 1
	use(y)
	return nil
}
// three`, 4, 4, `
func b() error {
	x := 1
	use(y)
	return nil
}
// one
// three`, note.Placement{Where: note.Likely, Start: 4, End: 4}},
		"a block that stood twice, the original deleted": {twice + twice + "end", 12, 13,
			twice + "end", note.Placement{Where: note.Ambiguous}},
		"a block that stood twice, the file changed elsewhere": {twice + twice + "end", 12, 13,
			"start\n" + twice + twice + "end", note.Placement{Where: note.Likely, Start: 13, End: 14}},
		"changed within": {twoReturns, 3, 4, strings.Replace(twoReturns, "use(x)", "use(x, x)", 1),
			note.Placement{Where: note.Changed, Start: 3, End: 4}},
		"deleted entirely": {twoReturns, 3, 4, strings.Replace(twoReturns, "\tx := 1\n\tuse(x)\n", "", 1),
			note.Placement{Where: note.Orphaned}},
		"changed within, the same text elsewhere": {"}\na\nb\nc\n}\nd\ne\nf", 5, 5, "}\na\nb\nc\n})\nd\ne\nf",
			note.Placement{Where: note.Changed, Start: 5, End: 5}},
		"the first line changed":      {"a\nb\nc\nd", 1, 1, "A\nb\nc\nd", note.Placement{Where: note.Changed, Start: 1, End: 1}},
		"the last line changed":       {"a\nb\nc\nd", 4, 4, "a\nb\nc\nD", note.Placement{Where: note.Changed, Start: 4, End: 4}},
		"lines inserted at the start": {"a\nb\nc\nd", 1, 2, "x\na\nb\nc\nd", note.Placement{Where: note.Exact, Start: 2, End: 3}},
		"reindented with its context": {twoReturns, 4, 4, strings.ReplaceAll(twoReturns, "\t", "  "),
			note.Placement{Where: note.Orphaned}},
		"a copy pasted with its context": {"x\na\nb\ny\nz", 2, 2, "x\na\nb\ny\nz\nx\na\nb\ny\nz",
			note.Placement{Where: note.Ambiguous}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			a := anchor(t, file(tt.before), tt.start, tt.end)
			assert.Equal(t, tt.want, note.Locate(a, change.ObjectID{}, file(tt.after)))
		})
	}
}

func TestTheSameBlobNeedsNoSearch(t *testing.T) {
	blob, err := change.ParseObjectID(strings.Repeat("ab", 20))
	require.NoError(t, err)
	twice := file("s\ns\ns\nA\ne\ne\ne\ns\ns\ns\nA\ne\ne\ne")
	a, err := note.NewAnchor("f", note.Origin{Kind: change.KindIndex}, blob, twice, 11, 11)
	require.NoError(t, err)
	assert.Equal(t, 2, a.Copies)
	assert.Equal(t, 2, a.Ordinal)
	assert.Equal(t, note.Placement{Where: note.Exact, Start: 11, End: 11}, note.Locate(a, blob, twice))
	assert.Equal(t, note.Placement{Where: note.Likely, Start: 11, End: 11}, note.Locate(a, change.ObjectID{}, twice),
		"by its text alone a copy is only likely")
}

func TestNewAnchorKeepsSelectionAndContext(t *testing.T) {
	lines := file("1\n2\n3\n4\n5\n6\n7\n8\n9")
	a := anchor(t, lines, 5, 6)
	assert.Equal(t, []string{"5", "6"}, a.Lines)
	assert.Equal(t, []string{"2", "3", "4"}, a.Before)
	assert.Equal(t, []string{"7", "8", "9"}, a.After)
	assert.Equal(t, 1, a.Copies)
	assert.Equal(t, 1, a.Ordinal)

	a = anchor(t, lines, 1, 1)
	assert.Empty(t, a.Before)

	_, err := note.NewAnchor("f", note.Origin{}, change.ObjectID{}, lines, 3, 2)
	require.ErrorIs(t, err, note.ErrInvalidRange)
	_, err = note.NewAnchor("f", note.Origin{}, change.ObjectID{}, lines, 1, 10)
	require.ErrorIs(t, err, note.ErrInvalidRange)
	long := slices.Repeat([]string{"x"}, note.MaxLines+1)
	_, err = note.NewAnchor("f", note.Origin{}, change.ObjectID{}, long, 1, len(long))
	require.ErrorIs(t, err, note.ErrTooManyLines)
}

// TestLocateNeverShowsAPlaceWithoutTheSelection draws files and
// selections from few distinct lines, so copies are common. It assumes
// nothing about which copy is meant, only that a place shown is right:
// unchanged, a unique selection stands where it was; and what Exact or
// Likely shows holds the selection, and what Changed shows lies between
// the context.
func TestLocateNeverShowsAPlaceWithoutTheSelection(t *testing.T) {
	line := rapid.SampledFrom([]string{"a", "b", "c", "}", "return nil"})
	rapid.Check(t, func(t *rapid.T) {
		lines := rapid.SliceOfN(line, 1, 30).Draw(t, "lines")
		start := rapid.IntRange(1, len(lines)).Draw(t, "start")
		end := rapid.IntRange(start, min(len(lines), start+4)).Draw(t, "end")
		a, err := note.NewAnchor("f", note.Origin{Kind: change.KindWorktree}, change.ObjectID{}, lines, start, end)
		if err != nil {
			t.Fatal(err)
		}
		if p := note.Locate(a, change.ObjectID{}, lines); a.Copies == 1 && p != (note.Placement{Where: note.Exact, Start: start, End: end}) {
			t.Fatalf("unchanged and unique, placed at %+v", p)
		}
		later := rapid.SliceOfN(line, 0, 30).Draw(t, "later")
		p := note.Locate(a, change.ObjectID{}, later)
		switch p.Where {
		case note.Exact, note.Likely:
			if !slices.Equal(later[p.Start-1:p.End], a.Lines) {
				t.Fatalf("%v at %d-%d without the selection", p.Where, p.Start, p.End)
			}
		case note.Changed:
			if p.Start < 1 || p.End < p.Start || p.End > len(later) {
				t.Fatalf("changed at %d-%d of %d lines", p.Start, p.End, len(later))
			}
		case note.Ambiguous, note.Orphaned:
		default:
			t.Fatalf("no outcome")
		}
	})
}
