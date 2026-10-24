package main

import (
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"seam/internal/adapter/driven/git"
	"seam/internal/app/usecase"
	"seam/internal/config"
	"seam/internal/observability"
)

// options is the graph of one session. It is the only place that knows use
// cases and adapters at once.
func options(cfg config.Config, logger *observability.Logger, root string) []fx.Option {
	return []fx.Option{
		fx.Supply(cfg, logger, git.Config{Root: root}, cfg.FileSizeLimit()),
		observability.Module,
		git.Module,
		usecase.Module,
		// fx.WithLogger insists on the interface type; NewFxLogger returns
		// the concrete one.
		fx.WithLogger(func(l *observability.Logger) fxevent.Logger { return observability.NewFxLogger(l) }),
	}
}
