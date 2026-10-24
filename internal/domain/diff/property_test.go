package diff_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"seam/internal/domain/diff"
)

// side draws a short file from few distinct lines, so that sides share many
// lines and the search has choices to make. "d" lacks a newline on purpose.
func side(t *rapid.T, label string) diff.Side {
	lines := rapid.SliceOfN(rapid.SampledFrom([]string{"a\n", "b\n", "c\n", "d"}), 0, 40).Draw(t, label)
	return diff.Side{Lines: lines}
}

// lcs is the oracle: the length of a longest common subsequence by dynamic
// programming, quadratic but beyond doubt.
func lcs(a, b []string) int {
	prev, cur := make([]int, len(b)+1), make([]int, len(b)+1)
	for i := range a {
		for j := range b {
			if a[i] == b[j] {
				cur[j+1] = prev[j] + 1
			} else {
				cur[j+1] = max(cur[j], prev[j+1])
			}
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// checkValid asserts what every result must hold, simplified or not.
func checkValid(t require.TestingT, d diff.FileDiff) {
	nextLeft, nextRight := 0, 0
	for i, r := range d.Rows {
		if r.Left >= 0 {
			require.Equal(t, nextLeft, r.Left, "row %d: old lines appear once, in order", i)
			nextLeft++
		}
		if r.Right >= 0 {
			require.Equal(t, nextRight, r.Right, "row %d: new lines appear once, in order", i)
			nextRight++
		}
		switch r.Kind {
		case diff.Equal:
			require.Equal(t, d.Old.Lines[r.Left], d.New.Lines[r.Right], "row %d: equal rows hold equal lines", i)
		case diff.Change:
			require.True(t, r.Left >= 0 && r.Right >= 0, "row %d: a change pairs two lines", i)
		case diff.Delete:
			require.True(t, r.Left >= 0 && r.Right < 0, "row %d", i)
		case diff.Insert:
			require.True(t, r.Left < 0 && r.Right >= 0, "row %d", i)
		default:
			require.Fail(t, "invalid kind", "row %d", i)
		}
	}
	require.Equal(t, len(d.Old.Lines), nextLeft)
	require.Equal(t, len(d.New.Lines), nextRight)

	// Changes are exactly the maximal runs of rows that are not Equal.
	var want []diff.RowRange
	for i := 0; i < len(d.Rows); i++ {
		if d.Rows[i].Kind == diff.Equal {
			continue
		}
		start := i
		for i < len(d.Rows) && d.Rows[i].Kind != diff.Equal {
			i++
		}
		want = append(want, diff.RowRange{Start: start, End: i})
	}
	require.Equal(t, want, d.Changes)
}

func equalRows(d diff.FileDiff) int {
	n := 0
	for _, r := range d.Rows {
		if r.Kind == diff.Equal {
			n++
		}
	}
	return n
}

func TestDiffIsValidAndMinimal(t *testing.T) {
	ctx := t.Context()
	rapid.Check(t, func(t *rapid.T) {
		old, new := side(t, "old"), side(t, "new")

		d := diff.Compute(ctx, old, new)

		checkValid(t, d)
		require.False(t, d.Simplified)
		require.Equal(t, lcs(old.Lines, new.Lines), equalRows(d), "as many equal rows as the oracle's LCS")
	})
}

// countdown is a context that ends after a number of checks, so that a test
// can stop the search at any point without depending on the clock.
type countdown struct {
	context.Context
	left int
}

func (c *countdown) Err() error {
	if c.left <= 0 {
		return context.Canceled
	}
	c.left--
	return nil
}

func TestCancelledDiffIsValidButCoarser(t *testing.T) {
	parent := t.Context()
	rapid.Check(t, func(t *rapid.T) {
		old, new := side(t, "old"), side(t, "new")
		ctx := &countdown{Context: parent, left: rapid.IntRange(0, 30).Draw(t, "checks")}

		d := diff.Compute(ctx, old, new)

		checkValid(t, d)
		want := lcs(old.Lines, new.Lines)
		require.LessOrEqual(t, equalRows(d), want)
		if !d.Simplified {
			require.Equal(t, want, equalRows(d), "unless simplified, the diff is minimal")
		}
	})
}

// bigSides are two files of n lines from a large vocabulary, every tenth
// line changed.
func bigSides(n int) (old, new diff.Side) {
	r := rand.New(rand.NewPCG(1, 2))
	for i := range n {
		line := fmt.Sprintf("line %d\n", r.IntN(n))
		old.Lines = append(old.Lines, line)
		if i%10 == 0 {
			line = fmt.Sprintf("changed %d\n", i)
		}
		new.Lines = append(new.Lines, line)
	}
	return old, new
}

func TestBudgetEndsTheSearchInTime(t *testing.T) {
	// Unrelated files make the search quadratic: seconds at this size.
	r := rand.New(rand.NewPCG(3, 4))
	var old, new diff.Side
	for range 50000 {
		old.Lines = append(old.Lines, fmt.Sprintf("%d\n", r.IntN(1000)))
		new.Lines = append(new.Lines, fmt.Sprintf("%d\n", r.IntN(1000)))
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	start := time.Now()
	d := diff.Compute(ctx, old, new)

	assert.Less(t, time.Since(start), time.Second)
	assert.True(t, d.Simplified)
	checkValid(t, d)
}

func TestNoCommonLineIsExactWithoutSearch(t *testing.T) {
	var old, new diff.Side
	for i := range 100000 {
		old.Lines = append(old.Lines, fmt.Sprintf("%d\r\n", i))
		new.Lines = append(new.Lines, fmt.Sprintf("%d\n", i))
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	d := diff.Compute(ctx, old, new)

	assert.False(t, d.Simplified, "a cancelled context must not matter when there is nothing to search")
	assert.Equal(t, []diff.RowRange{{Start: 0, End: 100000}}, d.Changes)
}

func BenchmarkCompute10kLines(b *testing.B) {
	old, new := bigSides(10000)
	for b.Loop() {
		diff.Compute(b.Context(), old, new)
	}
}
