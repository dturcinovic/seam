package fswatch

import (
	"errors"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTheLimitsAreNamed(t *testing.T) {
	assert.EqualError(t, limitHint(syscall.ENOSPC), "no space left on device (raise fs.inotify.max_user_watches)")
	assert.ErrorIs(t, limitHint(syscall.ENOSPC), syscall.ENOSPC)
	assert.EqualError(t, limitHint(syscall.EMFILE), "too many open files (raise fs.inotify.max_user_instances)")
	other := errors.New("other")
	assert.Equal(t, other, limitHint(other))
}
