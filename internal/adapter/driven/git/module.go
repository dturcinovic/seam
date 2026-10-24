package git

import (
	"go.uber.org/fx"

	"seam/internal/app/port/out"
	"seam/internal/supervise"
)

// Module provides the repository as the ports it fulfils. Its stop hook is
// the last to run, as everything that reads blobs is built on it.
var Module = fx.Module("git",
	fx.Provide(fx.Annotate(newRepo,
		fx.As(new(out.RevisionResolver)),
		fx.As(new(out.ChangeLister)),
		fx.As(new(out.UntrackedLister)),
		fx.As(new(out.BlobReader)),
		fx.As(new(out.WorktreeReader)),
		fx.As(new(out.WorktreeStatter)),
	)),
)

func newRepo(lc fx.Lifecycle, cfg Config, panics supervise.PanicFunc) (*Repo, error) {
	r, err := Open(cfg.Root, panics)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.StopHook(r.Close))
	return r, nil
}
