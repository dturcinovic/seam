package main

import (
	"cmp"
	"context"
	"path/filepath"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"github.com/dturcinovic/seam/internal/adapter/driven/chroma"
	"github.com/dturcinovic/seam/internal/adapter/driven/configfile"
	"github.com/dturcinovic/seam/internal/adapter/driven/files"
	"github.com/dturcinovic/seam/internal/adapter/driven/fswatch"
	"github.com/dturcinovic/seam/internal/adapter/driven/git"
	"github.com/dturcinovic/seam/internal/adapter/driven/github"
	"github.com/dturcinovic/seam/internal/adapter/driven/lsp"
	"github.com/dturcinovic/seam/internal/adapter/driven/notestore"
	"github.com/dturcinovic/seam/internal/adapter/driven/process"
	"github.com/dturcinovic/seam/internal/adapter/driven/statefile"
	"github.com/dturcinovic/seam/internal/app/port/out"
	"github.com/dturcinovic/seam/internal/app/usecase"
	"github.com/dturcinovic/seam/internal/config"
	"github.com/dturcinovic/seam/internal/domain/release"
	"github.com/dturcinovic/seam/internal/observability"
)

// options is the graph of one session. It is the only place that knows use
// cases and adapters at once. The language servers' stderr goes next to the
// log. root is empty outside a repository, where the working directory
// stands in for it: only files on disk are compared there, without hovers,
// and notes, which notes names the file of, are missing as well.
func options(cfg config.Config, logger *observability.Logger, root, notes string, file configfile.File) []fx.Option {
	running := thisBuild().release()
	dir := cmp.Or(root, cfg.Dir)
	return []fx.Option{
		fx.Supply(cfg, logger, git.Config{Root: dir, Checkouts: cfg.Checkouts}, lsp.Config{Root: dir, LogDir: filepath.Dir(cfg.LogFile), MaxInstances: file.MaxInstances}, cfg.FileSizeLimit(), file.Languages, files.Config{Root: root}),
		fx.Supply(usecase.ReleaseSettings{Running: running, Ask: cfg.UpdateCheck}, statefile.Config{ReleaseFile: cfg.ReleaseFile},
			github.Config{BaseURL: cfg.UpdateURL, Repo: repo, Agent: "seam/" + running.String()},
			notestore.Config{File: notes}, usecase.NotesSettings{File: notes}),
		observability.Module,
		git.Module,
		files.Module,
		chroma.Module,
		lsp.Module,
		fswatch.Module,
		statefile.Module,
		notestore.Module,
		process.Module,
		github.Module,
		usecase.Module,
		// fx stops in the reverse order of construction. Nothing ties the
		// language servers to git, so this builds git and its checkouts
		// first: the servers, which run in the checkouts, stop before
		// them, and git last, with the time the servers leave, whatever
		// order a caller asks for the ports in.
		fx.Invoke(func(out.BlobReader, out.CommitCheckouts, out.DocumentSync) {}),
		fxLogger(),
	}
}

// fxLogger logs fx's own events to the session's log. fx.WithLogger
// insists on the interface type; NewFxLogger returns the concrete one.
func fxLogger() fx.Option {
	return fx.WithLogger(func(l *observability.Logger) fxevent.Logger { return observability.NewFxLogger(l) })
}

// releaseNotice is the notice --version adds, from the last check alone: it
// builds no graph and asks no one. Whatever cannot be read only costs the
// notice, so it stops as soon as there can be none.
func releaseNotice(ctx context.Context, flags config.Flags, running release.Version) (string, bool) {
	if running.IsZero() {
		return "", false
	}
	cfg, err := config.Load(flags)
	if err != nil || !cfg.UpdateCheck {
		return "", false
	}
	file, err := configfile.Load(cfg.ConfigFile)
	if err != nil {
		return "", false
	}
	cfg = cfg.Settle(file.MaxFileSize, file.UpdateCheck)
	settings := usecase.ReleaseSettings{Running: running, Ask: cfg.UpdateCheck}
	latest, ok := usecase.NewerRelease(ctx, settings, statefile.New(statefile.Config{ReleaseFile: cfg.ReleaseFile}))
	if !ok {
		return "", false
	}
	return release.Notice(running, latest, updateCommand()), true
}
