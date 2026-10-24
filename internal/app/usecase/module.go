package usecase

import (
	"go.uber.org/fx"

	"seam/internal/app/port/in"
)

// Module provides the use cases as their incoming ports.
var Module = fx.Module("usecase",
	fx.Provide(
		fx.Annotate(NewChangesets, fx.As(new(in.ChangesetLoader))),
	),
)
