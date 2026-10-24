package diff

import (
	"slices"
	"strings"
)

// Side is one version of a file as lines. Each line keeps its terminator,
// "\n" or "\r\n"; the last line may have none. That way a missing newline at
// the end and a change of line endings are differences like any other.
type Side struct {
	Lines []string
}

// Content is a line without its terminator: what is shown, and what a Span
// counts in.
func Content(line string) string {
	if c, ok := strings.CutSuffix(line, "\r\n"); ok {
		return c
	}
	return strings.TrimSuffix(line, "\n")
}

// Split cuts text into lines after each "\n".
func Split(text string) Side {
	return Side{Lines: slices.Collect(strings.Lines(text))}
}
