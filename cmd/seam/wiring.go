package main

import (
	"path/filepath"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"github.com/dturcinovic/seam/internal/adapter/driven/chroma"
	"github.com/dturcinovic/seam/internal/adapter/driven/configfile"
	"github.com/dturcinovic/seam/internal/adapter/driven/fswatch"
	"github.com/dturcinovic/seam/internal/adapter/driven/git"
	"github.com/dturcinovic/seam/internal/adapter/driven/lsp"
	"github.com/dturcinovic/seam/internal/app/port/out"
	"github.com/dturcinovic/seam/internal/app/usecase"
	"github.com/dturcinovic/seam/internal/config"
	"github.com/dturcinovic/seam/internal/observability"
)

// options is the graph of one session. It is the only place that knows use
// cases and adapters at once. The language servers' stderr goes next to the
// log.
func options(cfg config.Config, logger *observability.Logger, root string, file configfile.File) []fx.Option {
	return []fx.Option{
		fx.Supply(cfg, logger, git.Config{Root: root, Checkouts: cfg.Checkouts}, lsp.Config{Root: root, LogDir: filepath.Dir(cfg.LogFile), MaxInstances: file.MaxInstances}, cfg.FileSizeLimit(), file.Languages),
		observability.Module,
		git.Module,
		chroma.Module,
		lsp.Module,
		fswatch.Module,
		usecase.Module,
		// fx stops in the reverse order of construction. Nothing ties the
		// language servers to git, so this builds git and its checkouts
		// first: the servers, which run in the checkouts, stop before
		// them, and git last, with the time the servers leave, whatever
		// order a caller asks for the ports in.
		fx.Invoke(func(out.BlobReader, out.CommitCheckouts, out.DocumentSync) {}),
		// fx.WithLogger insists on the interface type; NewFxLogger returns
		// the concrete one.
		fx.WithLogger(func(l *observability.Logger) fxevent.Logger { return observability.NewFxLogger(l) }),
	}
}
