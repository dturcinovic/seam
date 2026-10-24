package tui

import (
	"fmt"
	"strings"

	"seam/internal/domain/filediff"
	"seam/internal/domain/view"
)

// keys is the footer's reminder of the keys.
const keys = "q quit  r reload  f full/hunks  n/p change  tab/shift+tab file  j/k scroll  h/l sideways"

// header names the file, where it stands in the list, the view and how many
// differences it has, as IntelliJ's diff does.
func header(s *state) string {
	f, ok := s.file()
	if !ok {
		return "no changes"
	}
	// textcell replaces all a path may hold, but line breaks show as ␤ in
	// names, as the plan has it.
	path := strings.ReplaceAll(f.Path(), "\n", "␤")
	h := fmt.Sprintf("%d/%d  %s %s  %s", s.Current+1, len(s.Files), f.Status, path, modeName(s.Mode))
	switch {
	case !s.Loaded && s.Err != nil:
		return h + "  failed"
	case !s.Loaded:
		return h + "  loading…"
	}
	h += fmt.Sprintf("  %d differences", len(s.Snapshot.Diff.Changes))
	for _, n := range s.Snapshot.Notices {
		h += "  " + noticeText(n)
	}
	return h
}

// placeholder stands in the first line when there are no lines to show.
func placeholder(s *state) string {
	switch {
	case len(s.Files) == 0:
		return "nothing to compare"
	case !s.Loaded:
		return ""
	case len(s.Snapshot.Notices) > 0 && len(s.Snapshot.Diff.Rows) == 0:
		return "not shown as text"
	}
	return "no lines"
}

// footer is the last error, or else the keys.
func footer(s *state) (string, style) {
	if s.Err != nil {
		return "error: " + s.Err.Error(), errorStyle
	}
	return keys, footerStyle
}

func modeName(m view.Mode) string {
	if m == view.Full {
		return "full file"
	}
	return "hunks"
}

func noticeText(n filediff.Notice) string {
	side := ""
	switch n.Side {
	case filediff.OldSide:
		side = "old side "
	case filediff.NewSide:
		side = "new side "
	}
	switch n.Kind {
	case filediff.TooLarge:
		return fmt.Sprintf("%stoo large (%d bytes)", side, n.Size)
	case filediff.Binary:
		return side + "binary"
	case filediff.Simplified:
		return "diff simplified"
	case filediff.HighlightStopped:
		return fmt.Sprintf("%sno syntax colours from line %d", side, n.Line)
	}
	return ""
}
