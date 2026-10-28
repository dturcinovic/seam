package fswatch

import (
	"errors"
	"fmt"
	"syscall"
)

// limitHint names the limit to raise when the process runs out of file
// descriptors: kqueue, which fsnotify uses on macOS, takes one per watched
// directory and file. Not ulimit -n: Go raises the soft limit to what the
// system allows a process when it starts.
func limitHint(err error) error {
	switch {
	case errors.Is(err, syscall.EMFILE):
		return fmt.Errorf("%w (raise kern.maxfilesperproc)", err)
	case errors.Is(err, syscall.ENFILE):
		return fmt.Errorf("%w (raise kern.maxfiles)", err)
	}
	return err
}
