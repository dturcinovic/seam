package view

// Pos is where a view stands. AnchorRow is the row the view keeps across
// a change of view, and AnchorHalf its half, as the unified view may show
// a row twice; Scroll is the index of the top line. Switch derives Scroll
// from the anchor, Navigate the anchor from Scroll. The scroll position
// alone would not do: a row the hunk view folds away would be lost on the
// way from the full view and back.
type Pos struct {
	AnchorRow  int
	AnchorHalf Half
	Scroll     int
}

// Switch places a view whose lines changed, after a change of mode or
// context or layout: the anchor stays, and the top line becomes the one
// that holds it, within the bounds of the lines.
func Switch(lines []Line, pos Pos, height int) Pos {
	pos.Scroll = clampScroll(len(lines), LineFor(lines, pos.AnchorRow, pos.AnchorHalf), height)
	return pos
}

// Navigate moves the top line to scroll, within the bounds of the lines, and
// anchors the view at the row of the new top line; pos must be placed on
// these lines. The anchor stays when the view did not move, as at the end of
// the file, or when a fold at the top holds it: the screen shows the same
// place, so the place must not change.
func Navigate(lines []Line, pos Pos, scroll, height int) Pos {
	if len(lines) == 0 {
		return Pos{AnchorRow: pos.AnchorRow}
	}
	scroll = clampScroll(len(lines), scroll, height)
	moved := scroll != pos.Scroll
	pos.Scroll = scroll
	if top := lines[scroll]; moved && (!top.Fold || !top.Contains(pos.AnchorRow)) {
		pos.AnchorRow, pos.AnchorHalf = RowAt(lines, scroll), top.Half
	}
	return pos
}

// clampScroll keeps the last page full: the top line of n stops where the
// last line reaches the bottom of height lines. Only Scroll is bounded,
// never the anchor, so that a view that is shorter for a while gives the
// place back.
func clampScroll(n, scroll, height int) int {
	return max(min(scroll, n-max(height, 1)), 0)
}
