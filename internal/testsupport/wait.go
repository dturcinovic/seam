package testsupport

import (
	"testing"
	"time"
)

// Guard bounds every wait of a test, so a lost signal fails rather than
// hangs.
const Guard = 5 * time.Second

// Updates is what the waits need of a port that hands out updates.
type Updates[U any] interface {
	Signal() <-chan struct{}
	Drain() []U
}

// Collect drains p on each signal until done says the updates so far are
// complete, and returns them all.
func Collect[U any](t testing.TB, p Updates[U], done func([]U) bool) []U {
	t.Helper()
	var all []U
	timeout := time.After(Guard)
	for !done(all) {
		select {
		case <-p.Signal():
			all = append(all, p.Drain()...)
		case <-timeout:
			t.Fatalf("no complete updates within %v, got %d", Guard, len(all))
		}
	}
	return all
}

// Await returns the value ch delivers.
func Await[T any](t testing.TB, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(Guard):
		t.Fatalf("no value within %v", Guard)
		var zero T
		return zero
	}
}

// Until returns once ch is closed or receives.
func Until(t testing.TB, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(Guard):
		t.Fatalf("not reached within %v", Guard)
	}
}
