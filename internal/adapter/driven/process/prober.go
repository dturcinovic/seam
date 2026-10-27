package process

import (
	"errors"
	"syscall"
)

// Prober implements out.ProcessProber.
type Prober struct{}

// New returns the prober.
func New() Prober { return Prober{} }

// Alive implements out.ProcessProber with the signal 0, which checks
// without sending: a process of another user still runs where the check
// is refused. A pid reused since counts as alive; that only keeps a
// session in the book longer.
func (Prober) Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
