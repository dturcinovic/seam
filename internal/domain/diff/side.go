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

// Split cuts text into lines after each "\n".
func Split(text string) Side {
	return Side{Lines: slices.Collect(strings.Lines(text))}
}
