package diff_test

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/diff"
)

// render shows rows compactly: "=a|a" equal, "~x|y" changed pair, "-x|"
// deleted, "+|y" inserted; line terminators are left out.
func render(d diff.FileDiff) []string {
	text := func(lines []string, i int) string {
		if i < 0 {
			return ""
		}
		return diff.Content(lines[i])
	}
	marks := map[diff.RowKind]string{diff.Equal: "=", diff.Change: "~", diff.Delete: "-", diff.Insert: "+"}
	var out []string
	for _, r := range d.Rows {
		out = append(out, marks[r.Kind]+text(d.Old.Lines, r.Left)+"|"+text(d.New.Lines, r.Right))
	}
	return out
}

func TestCompute(t *testing.T) {
	tests := []struct {
		name     string
		old, new string
		want     []string
		blocks   int
	}{
		{"pure insert", "a\nc\n", "a\nb\nc\n", []string{"=a|a", "+|b", "=c|c"}, 1},
		{"pure delete", "a\nb\nc\n", "a\nc\n", []string{"=a|a", "-b|", "=c|c"}, 1},
		{"replace 2:1", "a\nx\ny\nb\n", "a\nz\nb\n", []string{"=a|a", "~x|z", "-y|", "=b|b"}, 1},
		{"replace 1:3", "a\nx\nb\n", "a\np\nq\nr\nb\n", []string{"=a|a", "~x|p", "+|q", "+|r", "=b|b"}, 1},
		{"two blocks", "a\nx\nb\ny\nc\n", "a\nX\nb\nY\nc\n", []string{"=a|a", "~x|X", "=b|b", "~y|Y", "=c|c"}, 2},
		{"both empty", "", "", nil, 0},
		{"old empty", "", "a\nb\n", []string{"+|a", "+|b"}, 1},
		{"new empty", "a\nb\n", "", []string{"-a|", "-b|"}, 1},
		{"no change", "a\nb\n", "a\nb\n", []string{"=a|a", "=b|b"}, 0},
		{"newline added at the end", "a\nb", "a\nb\n", []string{"=a|a", "~b|b"}, 1},
		{"CRLF to LF", "a\r\nb\r\n", "a\nb\n", []string{"~a|a", "~b|b"}, 1},
		{"nothing in common", "a\nb\n", "c\n", []string{"~a|c", "-b|"}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := diff.Compute(t.Context(), diff.Split(tt.old), diff.Split(tt.new))

			assert.Equal(t, tt.want, render(d))
			assert.Len(t, d.Changes, tt.blocks)
			assert.False(t, d.Simplified)
		})
	}
}

func TestChangesAreTheBlocks(t *testing.T) {
	d := diff.Compute(t.Context(), diff.Split("a\nx\nb\ny\nz\nc\n"), diff.Split("a\nb\nY\nc\nw\n"))

	assert.Equal(t, []diff.RowRange{{Start: 1, End: 2}, {Start: 3, End: 5}, {Start: 6, End: 7}}, d.Changes)
}

func TestContent(t *testing.T) {
	assert.Equal(t, "a", diff.Content("a\r\n"))
	assert.Equal(t, "a", diff.Content("a\n"))
	assert.Equal(t, "a", diff.Content("a"))
	assert.Equal(t, "a\r", diff.Content("a\r"), "a lone CR is content")
}

func TestSplit(t *testing.T) {
	assert.Empty(t, diff.Split("").Lines)
	assert.Equal(t, []string{"a\n", "b"}, diff.Split("a\nb").Lines)
	assert.Equal(t, []string{"a\r\n", "\n"}, diff.Split("a\r\n\n").Lines)
}

func TestApproxBytes(t *testing.T) {
	assert.Zero(t, diff.FileDiff{}.ApproxBytes())

	d := diff.Compute(t.Context(), diff.Split("a b c\n"), diff.Split("a x c\n"))
	require.Len(t, d.Inline, 1)
	unmarked := d
	unmarked.Inline = nil

	assert.Greater(t, unmarked.ApproxBytes(), int64(len("a b c\n")*2), "the text and more")
	entry := int64(unsafe.Sizeof(diff.InlineSpans{}) + 2*unsafe.Sizeof(diff.Span{}))
	assert.Equal(t, unmarked.ApproxBytes()+entry, d.ApproxBytes(), "one entry with a span a side")
}

func TestInlineAt(t *testing.T) {
	d := diff.Compute(t.Context(), diff.Split("same\na b c\nkeep\nd e f\n"), diff.Split("same\na x c\nkeep\nd e g\n"))
	require.Len(t, d.Inline, 2)

	for _, row := range []int{0, 2} {
		left, right := d.InlineAt(row)
		assert.Nil(t, left, "row %d has no marks", row)
		assert.Nil(t, right, "row %d has no marks", row)
	}
	left, right := d.InlineAt(3)
	assert.Equal(t, []diff.Span{{Start: 4, End: 5}}, left)
	assert.Equal(t, []diff.Span{{Start: 4, End: 5}}, right)
	left, _ = d.InlineAt(9)
	assert.Nil(t, left, "past the end")
}
