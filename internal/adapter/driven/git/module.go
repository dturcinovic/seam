package git

import (
	"context"
	"time"

	"go.uber.org/fx"
	"go.uber.org/zap"

	"seam/internal/app/port/out"
	"seam/internal/supervise"
)

// Module provides the repository and its checkouts as the ports they
// fulfil. The repository's stop hook is the last to run, as everything
// that reads blobs is built on it; the checkouts' runs just before, once
// the servers working in them have stopped.
var Module = fx.Module("git",
	fx.Provide(fx.Annotate(newRepo,
		fx.As(new(out.RevisionResolver)),
		fx.As(new(out.ChangeLister)),
		fx.As(new(out.UntrackedLister)),
		fx.As(new(out.BlobReader)),
		fx.As(new(out.WorktreeReader)),
		fx.As(new(out.WorktreeStatter)),
	)),
	fx.Provide(fx.Annotate(newCheckouts, fx.As(new(out.CommitCheckouts)))),
)

// repoStopReserve is what the checkouts leave of the stop deadline to the
// repository: closing cat-file takes at most 100 ms.
const repoStopReserve = 150 * time.Millisecond

func newRepo(lc fx.Lifecycle, cfg Config, panics supervise.PanicFunc) (*Repo, error) {
	r, err := Open(cfg.Root, panics)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.StopHook(r.Close))
	return r, nil
}

// newCheckouts depends on the repository, which fx provides only as its
// ports, so that it is built after it and stops before it.
func newCheckouts(lc fx.Lifecycle, cfg Config, _ out.BlobReader, log *zap.Logger, panics supervise.PanicFunc) *Checkouts {
	c := NewCheckouts(cfg.Root, cfg.Checkouts, log, panics)
	lc.Append(fx.StartStopHook(c.Start, func(ctx context.Context) error {
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) > repoStopReserve {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, deadline.Add(-repoStopReserve))
			defer cancel()
		}
		return c.Close(ctx)
	}))
	return c
}
