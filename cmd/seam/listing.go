package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dturcinovic/seam/internal/domain/change"
)

// printChangeset lists the files until the TUI shows them.
func printChangeset(w io.Writer, cs change.Changeset) error {
	var b strings.Builder
	for _, f := range cs.Files {
		path := printable(f.Path())
		if f.Status == change.Renamed || f.Status == change.Copied {
			path = printable(f.OldPath) + " -> " + path
		}
		fmt.Fprintf(&b, "%s\t%s\n", f.Status, path) // A strings.Builder does not fail.
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// printableLines applies printable to each line of a message, such as an
// error that names a path or passes on git's stderr.
func printableLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = printable(l)
	}
	return strings.Join(lines, "\n")
}

// printable quotes a path that holds anything a terminal would not show as
// is: control and format characters, escape sequences among them, and bytes
// that are no UTF-8.
func printable(s string) string {
	if !utf8.ValidString(s) || strings.IndexFunc(s, func(r rune) bool { return !unicode.IsGraphic(r) }) >= 0 {
		return strconv.QuoteToGraphic(s)
	}
	return s
}
