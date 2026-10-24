package view

// Unfold shows the rows of the fold at line i one line each, and returns
// lines and pos as they are for any other i. The top line keeps its row. A
// fold that held the anchor hands it to that row, as any navigation would:
// left inside the unfolded rows, it could lie far below the screen, and the
// next Switch would jump there.
func Unfold(lines []Line, pos Pos, i int) ([]Line, Pos) {
	if i < 0 || i >= len(lines) || !lines[i].Fold {
		return lines, pos
	}
	fold := lines[i]
	out := make([]Line, 0, len(lines)-1+fold.Rows.End-fold.Rows.Start)
	out = append(out, lines[:i]...)
	out = rowLines(out, fold.Rows.Start, fold.Rows.End)
	out = append(out, lines[i+1:]...)
	if i < pos.Scroll {
		pos.Scroll += len(out) - len(lines)
	}
	if fold.Contains(pos.AnchorRow) {
		pos.AnchorRow, pos.AnchorHalf = RowAt(out, pos.Scroll), out[pos.Scroll].Half
	}
	return out, pos
}
