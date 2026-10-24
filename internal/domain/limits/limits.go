package limits

import (
	"errors"
	"fmt"
	"math"
	"strconv"
)

// FileSize is the most bytes one side of a file may have to be loaded. The
// zero value is not a valid limit; use NewFileSize or DefaultFileSize.
type FileSize int64

// DefaultFileSize is the limit when none is configured. Up to this size a
// file gets the full treatment: diff, inline diff and highlighting.
const DefaultFileSize FileSize = 8 << 20

// ErrInvalidFileSize is returned for a limit that is not a positive size.
var ErrInvalidFileSize = errors.New("file size limit must be a positive size")

// NewFileSize checks n and returns it as a limit.
func NewFileSize(n int64) (FileSize, error) {
	if n <= 0 {
		return 0, fmt.Errorf("%w: %d", ErrInvalidFileSize, n)
	}
	return FileSize(n), nil
}

// Bytes is the limit as a byte count.
func (s FileSize) Bytes() int64 { return int64(s) }

// ParseFileSize reads a limit as written by a user: bytes, or a number with
// a binary suffix K, M or G, such as 16M.
func ParseFileSize(s string) (FileSize, error) {
	digits, shift := s, 0
	if n := len(s); n > 0 {
		switch s[n-1] {
		case 'K':
			digits, shift = s[:n-1], 10
		case 'M':
			digits, shift = s[:n-1], 20
		case 'G':
			digits, shift = s[:n-1], 30
		}
	}
	n, err := strconv.ParseInt(digits, 10, 64)
	// A negative number would wrap around in the shift.
	if err != nil || n <= 0 || n > math.MaxInt64>>shift {
		return 0, fmt.Errorf("%w: %q, want bytes or a number with K, M or G", ErrInvalidFileSize, s)
	}
	return NewFileSize(n << shift)
}
