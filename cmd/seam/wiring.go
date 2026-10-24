package main

import (
	"path/filepath"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"seam/internal/adapter/driven/chroma"
	"seam/internal/adapter/driven/git"
	"seam/internal/adapter/driven/lsp"
	"seam/internal/app/port/out"
	"seam/internal/app/usecase"
	"seam/internal/config"
	"seam/internal/domain/lang"
	"seam/internal/observability"
)

// options is the graph of one session. It is the only place that knows use
// cases and adapters at once. The language servers' stderr goes next to the
// log.
func options(cfg config.Config, logger *observability.Logger, root string, languages lang.Table) []fx.Option {
	return []fx.Option{
		fx.Supply(cfg, logger, git.Config{Root: root}, lsp.Config{Root: root, LogDir: filepath.Dir(cfg.LogFile)}, cfg.FileSizeLimit(), languages),
		observability.Module,
		git.Module,
		chroma.Module,
		lsp.Module,
		usecase.Module,
		// fx stops in the reverse order of construction. Nothing ties the
		// language servers to git, so this builds git first: it stops
		// last, with the time the servers leave, whatever order a caller
		// asks for the ports in.
		fx.Invoke(func(out.BlobReader, out.DocumentSync) {}),
		// fx.WithLogger insists on the interface type; NewFxLogger returns
		// the concrete one.
		fx.WithLogger(func(l *observability.Logger) fxevent.Logger { return observability.NewFxLogger(l) }),
	}
}
