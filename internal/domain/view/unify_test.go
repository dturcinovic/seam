package view_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"seam/internal/domain/diff"
	"seam/internal/domain/view"
)

// blockDiff is a diff of the given parts in order: a number n > 0 is n equal
// rows, a block {paired, rest} pairs paired rows and adds rest deleted rows,
// or -rest inserted ones, as diff aligns a block.
func blockDiff(parts ...[2]int) diff.FileDiff {
	var d diff.FileDiff
	add := func(k diff.RowKind, left, right bool) {
		r := diff.Row{Kind: k, Left: -1, Right: -1}
		if left {
			r.Left = len(d.Old.Lines)
			d.Old.Lines = append(d.Old.Lines, "x\n")
		}
		if right {
			r.Right = len(d.New.Lines)
			d.New.Lines = append(d.New.Lines, "x\n")
		}
		d.Rows = append(d.Rows, r)
	}
	for _, p := range parts {
		if p[1] == 0 && p[0] < 0 {
			for range -p[0] {
				add(diff.Equal, true, true)
			}
			continue
		}
		start := len(d.Rows)
		for range p[0] {
			add(diff.Change, true, true)
		}
		for range p[1] {
			add(diff.Delete, true, false)
		}
		for range -p[1] {
			add(diff.Insert, false, true)
		}
		d.Changes = append(d.Changes, diff.RowRange{Start: start, End: len(d.Rows)})
	}
	return d
}

// equal is n equal rows for blockDiff.
func equal(n int) [2]int { return [2]int{-n, 0} }

// renderHalves shows lines as render does, with "-" for an old and "+" for
// a new half.
func renderHalves(lines []view.Line) []string {
	out := render(lines)
	for i, l := range lines {
		switch l.Half {
		case view.OldHalf:
			out[i] += "-"
		case view.NewHalf:
			out[i] += "+"
		case view.Whole:
		}
	}
	return out
}

func TestUnify(t *testing.T) {
	tests := []struct {
		name string
		d    diff.FileDiff
		mode view.Mode
		want []string
	}{
		{"no change", blockDiff(equal(3)), view.Full, []string{"0", "1", "2"}},
		{"a changed row shows twice", blockDiff(equal(1), [2]int{1, 0}, equal(1)), view.Full,
			[]string{"0", "1-", "1+", "2"}},
		{"old halves first", blockDiff([2]int{2, 1}), view.Full,
			[]string{"0-", "1-", "2-", "0+", "1+"}},
		{"new halves last", blockDiff([2]int{1, -2}), view.Full,
			[]string{"0-", "0+", "1+", "2+"}},
		{"only deleted", blockDiff(equal(1), [2]int{0, 2}), view.Full, []string{"0", "1-", "2-"}},
		{"only inserted", blockDiff([2]int{0, -2}, equal(1)), view.Full, []string{"0+", "1+", "2"}},
		{"blocks stay apart", blockDiff([2]int{1, 0}, equal(1), [2]int{1, 0}), view.Full,
			[]string{"0-", "0+", "1", "2-", "2+"}},
		{"folds stay", blockDiff(equal(5), [2]int{1, 0}, equal(5)), view.Hunks,
			[]string{"f0:4", "4", "5-", "5+", "6", "f7:11"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, renderHalves(view.Unify(tt.d, view.Lines(tt.d, tt.mode, 1))))
		})
	}
}

func TestLineForRowInTheUnifiedView(t *testing.T) {
	// 0, 1-, 2-, 3-, 1+, 2+, 4, 5-, 5+, 6+, 7
	d := blockDiff(equal(1), [2]int{2, 1}, equal(1), [2]int{1, -1}, equal(1))
	lines := view.Unify(d, view.Lines(d, view.Full, 0))

	for row, want := range []int{0, 1, 2, 3, 6, 7, 9, 10} {
		assert.Equal(t, want, view.LineForRow(lines, row), "row %d", row)
	}
	assert.Equal(t, 4, view.LineFor(lines, 1, view.NewHalf))
	assert.Equal(t, 1, view.LineFor(lines, 1, view.OldHalf))
	assert.Equal(t, 3, view.LineFor(lines, 3, view.NewHalf), "a deleted row has no new half")
	assert.Equal(t, 9, view.LineFor(lines, 6, view.NewHalf), "an inserted row has only it")
	assert.Equal(t, 6, view.LineFor(lines, 4, view.NewHalf), "an equal row is whole")
}

// randomBlocks draws equal rows and blocks of every shape diff makes.
func randomBlocks(t *rapid.T) diff.FileDiff {
	var parts [][2]int
	for i := range rapid.IntRange(0, 8).Draw(t, "parts") {
		if i%2 == 0 {
			parts = append(parts, equal(rapid.IntRange(1, 6).Draw(t, "equal")))
			continue
		}
		b := [2]int{rapid.IntRange(0, 4).Draw(t, "paired"), rapid.IntRange(-4, 4).Draw(t, "rest")}
		if b != [2]int{} {
			parts = append(parts, b)
		}
	}
	return blockDiff(parts...)
}

func TestUnifyShowsEveryHalfOnce(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		d := randomBlocks(t)
		mode := rapid.SampledFrom([]view.Mode{view.Hunks, view.Full}).Draw(t, "mode")
		lines := view.Unify(d, view.Lines(d, mode, rapid.IntRange(0, 3).Draw(t, "context")))

		seen := map[string]int{}
		for _, l := range lines {
			for row := l.Rows.Start; row < l.Rows.End; row++ {
				seen[fmt.Sprint(row, l.Half)]++
			}
		}
		for row, r := range d.Rows {
			if r.Kind == diff.Equal {
				require.Equal(t, 1, seen[fmt.Sprint(row, view.Whole)], "row %d", row)
				continue
			}
			require.Equal(t, btoi(r.Left >= 0), seen[fmt.Sprint(row, view.OldHalf)], "old half of row %d", row)
			require.Equal(t, btoi(r.Right >= 0), seen[fmt.Sprint(row, view.NewHalf)], "new half of row %d", row)
		}
		for row := range d.Rows {
			i := view.LineForRow(lines, row)
			require.True(t, lines[i].Contains(row), "row %d", row)
			for j := range i {
				require.False(t, lines[j].Contains(row), "line %d holds row %d before line %d", j, row, i)
			}
		}
	})
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestSwitchingLayoutsKeepsTheTopHalf(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		d := randomBlocks(t)
		height := rapid.IntRange(1, 10).Draw(t, "height")
		split := view.Lines(d, view.Full, 0)
		unified := view.Unify(d, split)
		if len(unified) == 0 {
			return
		}

		pos := view.Navigate(unified, view.Pos{}, rapid.IntRange(0, len(unified)-1).Draw(t, "scroll"), height)
		top := pos.Scroll
		for range rapid.IntRange(1, 3).Draw(t, "round trips") {
			pos = view.Switch(split, pos, height)
			require.Equal(t, max(min(pos.AnchorRow, len(split)-height), 0), pos.Scroll, "split shows the row")
			pos = view.Switch(unified, pos, height)
		}
		require.Equal(t, top, pos.Scroll)
	})
}
