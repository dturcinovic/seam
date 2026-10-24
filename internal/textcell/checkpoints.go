package textcell

import (
	"context"

	"github.com/charmbracelet/x/ansi"
)

// Checkpoint is a grapheme boundary in a text, where a walk can resume: Byte
// is its offset, Col its column counted by code point and GraphemeCol
// counted by grapheme.
type Checkpoint struct {
	Byte, Col, GraphemeCol int
}

// Checkpoints returns the first grapheme boundary at or past each multiple
// of every bytes in text, with its column under both methods; none if every
// is not positive. It segments text once, as the graphemes are the same
// under both, and stops with ctx's error at the next checkpoint once ctx
// ends.
func Checkpoints(ctx context.Context, text string, every int) ([]Checkpoint, error) {
	if every <= 0 {
		return nil, nil
	}
	points := make([]Checkpoint, 0, len(text)/every)
	pos, col, gcol, next := 0, 0, 0, every
	for pos < len(text) {
		if pos >= next {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			points = append(points, Checkpoint{Byte: pos, Col: col, GraphemeCol: gcol})
			next = (pos/every + 1) * every
		}
		n, k, graphemes := cut(text[pos:], Graphemes)
		g := text[pos : pos+n]
		codepoints := graphemes
		if k == clusterKind {
			// Counting code points needs no segmenting.
			codepoints = ansi.WcWidth.StringWidth(g)
		}
		w, _ := widthOf(g, k, codepoints, col)
		gw, _ := widthOf(g, k, graphemes, gcol)
		col, gcol = col+w, gcol+gw
		pos += n
	}
	return points, nil
}
