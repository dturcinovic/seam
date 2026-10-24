package usecase_test

import (
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// guard bounds every wait of a test, so a lost signal fails rather than
// hangs.
const guard = 5 * time.Second

// loader is what the waits need of a use case's incoming port.
type loader[U any] interface {
	Signal() <-chan struct{}
	Drain() []U
}

// collect drains l on each signal until done says the updates so far are
// complete, and returns them all.
func collect[U any](t *testing.T, l loader[U], done func([]U) bool) []U {
	t.Helper()
	var all []U
	timeout := time.After(guard)
	for !done(all) {
		select {
		case <-l.Signal():
			all = append(all, l.Drain()...)
		case <-timeout:
			t.Fatalf("no complete updates within %v, got %d", guard, len(all))
		}
	}
	return all
}

// until returns once ch is closed or receives.
func until(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(guard):
		t.Fatalf("not reached within %v", guard)
	}
}
