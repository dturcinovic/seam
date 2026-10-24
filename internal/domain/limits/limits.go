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

// Instances is how many language server instances may run at once. The
// zero value is not a valid limit; use NewInstances or DefaultInstances.
type Instances int

// The bounds of Instances. Each instance is a server process with its own
// index of a workspace, which can take gigabytes.
const (
	DefaultInstances Instances = 4
	MaxInstances     Instances = 16
)

// ErrInvalidInstances is returned for a limit outside 1 to MaxInstances.
var ErrInvalidInstances = errors.New("instance limit out of bounds")

// OrDefault is n, or DefaultInstances if n is not set.
func (n Instances) OrDefault() Instances {
	if n <= 0 {
		return DefaultInstances
	}
	return n
}

// NewInstances checks n and returns it as a limit.
func NewInstances(n int) (Instances, error) {
	if n < 1 || n > int(MaxInstances) {
		return 0, fmt.Errorf("%w: %d, want 1 to %d", ErrInvalidInstances, n, MaxInstances)
	}
	return Instances(n), nil
}
