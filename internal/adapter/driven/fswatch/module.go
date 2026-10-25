package fswatch

import (
	"go.uber.org/fx"

	"github.com/dturcinovic/seam/internal/app/port/out"
)

// Module provides the watcher as out.ChangeWatcher. It holds nothing until
// Watch runs, and Watch ends with its context.
var Module = fx.Module("fswatch",
	fx.Provide(fx.Annotate(New, fx.As(new(out.ChangeWatcher)))),
)
