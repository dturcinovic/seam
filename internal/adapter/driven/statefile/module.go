package statefile

import (
	"go.uber.org/fx"

	"github.com/dturcinovic/seam/internal/app/port/out"
)

// Config is where the state lives.
type Config struct {
	// ReleaseFile holds the last check for a newer release.
	ReleaseFile string
}

// Module provides the store as out.ReleaseStore.
var Module = fx.Module("statefile",
	fx.Provide(fx.Annotate(New, fx.As(new(out.ReleaseStore)))),
)
