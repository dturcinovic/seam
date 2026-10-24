package limits

import (
	"errors"
	"fmt"
)

// FileSize is the most bytes one side of a file may have to be loaded. The
// zero value is not a valid limit; use NewFileSize or DefaultFileSize.
type FileSize int64

// DefaultFileSize is the limit when none is configured. Up to this size a
// file gets the full treatment: diff, inline diff and highlighting.
const DefaultFileSize FileSize = 8 << 20

// ErrInvalidFileSize is returned for a limit that is not positive.
var ErrInvalidFileSize = errors.New("file size limit must be positive")

// NewFileSize checks n and returns it as a limit.
func NewFileSize(n int64) (FileSize, error) {
	if n <= 0 {
		return 0, fmt.Errorf("%w: %d", ErrInvalidFileSize, n)
	}
	return FileSize(n), nil
}

// Bytes is the limit as a byte count.
func (s FileSize) Bytes() int64 { return int64(s) }
