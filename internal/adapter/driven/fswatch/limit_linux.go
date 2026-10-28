package fswatch

import (
	"errors"
	"fmt"
	"syscall"
)

// limitHint names the setting to raise when the kernel runs out of watches
// or watchers.
func limitHint(err error) error {
	switch {
	case errors.Is(err, syscall.ENOSPC):
		return fmt.Errorf("%w (raise fs.inotify.max_user_watches)", err)
	case errors.Is(err, syscall.EMFILE):
		return fmt.Errorf("%w (raise fs.inotify.max_user_instances)", err)
	}
	return err
}
