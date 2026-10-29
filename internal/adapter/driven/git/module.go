package git

import (
	"context"
	"time"

	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/dturcinovic/seam/internal/app/port/out"
	"github.com/dturcinovic/seam/internal/supervise"
)

// Module provides the repository, its checkouts and the reader of HEAD as
// the ports they fulfil; fx builds only what is asked for, so HEAD alone
// opens no repository. The repository's stop hook is the last to run, as
// everything that reads blobs is built on it; the checkouts' runs just
// before, once the servers working in them have stopped.
var Module = fx.Module("git",
	fx.Provide(fx.Annotate(newRepo,
		fx.As(new(out.RevisionResolver)),
		fx.As(new(out.ChangeLister)),
		fx.As(new(out.UntrackedLister)),
		fx.As(new(out.BlobReader)),
		fx.As(new(out.WorktreeReader)),
		fx.As(new(out.SubmoduleReader)),
		fx.As(new(out.WorktreeStatter)),
		fx.As(new(out.WatchPathsReader)),
		fx.As(new(out.IgnoreChecker)),
		fx.As(new(out.ObjectFormatReader)),
	)),
	fx.Provide(fx.Annotate(newCheckouts, fx.As(new(out.CommitCheckouts)))),
	fx.Provide(fx.Annotate(func(cfg Config) *Head { return NewHead(cfg.Root) }, fx.As(new(out.HeadReader)))),
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
