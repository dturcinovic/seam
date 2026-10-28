package fswatch

import (
	"errors"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTheLimitsAreNamed(t *testing.T) {
	assert.EqualError(t, limitHint(syscall.EMFILE), "too many open files (raise kern.maxfilesperproc)")
	assert.ErrorIs(t, limitHint(syscall.EMFILE), syscall.EMFILE)
	assert.EqualError(t, limitHint(syscall.ENFILE), "too many open files in system (raise kern.maxfiles)")
	other := errors.New("other")
	assert.Equal(t, other, limitHint(other))
}
