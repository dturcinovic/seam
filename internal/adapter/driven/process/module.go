package process

import (
	"go.uber.org/fx"

	"github.com/dturcinovic/seam/internal/app/port/out"
)

// Module provides the prober as out.ProcessProber.
var Module = fx.Module("process",
	fx.Provide(fx.Annotate(New, fx.As(new(out.ProcessProber)))),
)
