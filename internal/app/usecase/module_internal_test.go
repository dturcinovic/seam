package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
)

// stopRecorder is a worker that records the deadline it was stopped with.
type stopRecorder struct{ deadline time.Time }

func (*stopRecorder) Start() error { return nil }

func (r *stopRecorder) Stop(ctx context.Context) error {
	r.deadline, _ = ctx.Deadline()
	return nil
}

func TestRunWorkerLeavesTheReserve(t *testing.T) {
	tests := []struct {
		name        string
		left        time.Duration
		wantShorter bool
	}{
		{"ample time", time.Second, true},
		{"less than the reserve", stopReserve / 2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lc, w := fxtest.NewLifecycle(t), &stopRecorder{}
			runWorker(lc, w)
			require.NoError(t, lc.Start(t.Context()))
			ctx, cancel := context.WithTimeout(t.Context(), tt.left)
			defer cancel()
			deadline, _ := ctx.Deadline()

			require.NoError(t, lc.Stop(ctx))

			if tt.wantShorter {
				assert.Equal(t, deadline.Add(-stopReserve), w.deadline)
			} else {
				assert.Equal(t, deadline, w.deadline)
			}
		})
	}
}
