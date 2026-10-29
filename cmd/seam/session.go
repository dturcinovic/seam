package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/x/term"
	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/dturcinovic/seam/internal/adapter/driven/configfile"
	"github.com/dturcinovic/seam/internal/adapter/driven/git"
	"github.com/dturcinovic/seam/internal/adapter/driving/tui"
	"github.com/dturcinovic/seam/internal/app/port/in"
	"github.com/dturcinovic/seam/internal/app/usecase"
	"github.com/dturcinovic/seam/internal/config"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/release"
	"github.com/dturcinovic/seam/internal/domain/ui"
	"github.com/dturcinovic/seam/internal/observability"
	"github.com/dturcinovic/seam/internal/supervise"
)

// stopBudget bounds the whole shutdown, all OnStop hooks together.
const stopBudget = 3 * time.Second

// session finds the repository, builds the graph, loads the changeset of c,
// shows it, with watch reloading on every change, and stops the graph within
// stopBudget. It owns the logger and closes it only after the graph has
// stopped.
func session(ctx context.Context, cfg config.Config, c change.Comparison, watch bool, stdout, stderr io.Writer) (err error) {
	// Before the graph: it runs git, which ctx can cancel, and its errors
	// and those of the configuration file are the user's to fix rather
	// than faults of the wiring.
	// Files on disk need no repository, nor even git: the root is then
	// empty, and the reason is logged once there is a log.
	root, noRepo := git.FindRoot(ctx, cfg.Dir)
	if noRepo != nil && c.Left.Kind() != change.KindPath {
		return noRepo
	}
	file, err := configfile.Load(cfg.ConfigFile)
	if err != nil {
		return err
	}
	cfg = cfg.Settle(file.MaxFileSize, file.UpdateCheck)
	if err := createHeapProfile(cfg.HeapProfile); err != nil {
		return err
	}
	logger, err := observability.NewLogger(cfg)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, logger.Close()) }()
	if noRepo != nil {
		logger.Zap().Info("no repository: files on disk without hovers", zap.Error(noRepo))
	}
	notes := notesFiles(ctx, root, logger)

	var (
		changesets in.ChangesetLoader
		watches    in.WatchStarter
		releases   in.ReleaseNotifier
		ports      tui.Ports
		panics     supervise.PanicFunc
	)
	app := fx.New(append(options(cfg, logger, root, notes, file),
		fx.Populate(&changesets, &watches, &releases, &ports.Files, &ports.Stream, &ports.Reload, &ports.Docs, &ports.LSP, &ports.Hover, &ports.Define, &ports.Watch,
			&ports.NoteWriter, &ports.NoteReader, &ports.NoteOverview, &ports.NoteReleaser, &ports.NoteDeleter, &ports.NoteEditor, &ports.NoteSender, &ports.NoteUpdates, &panics))...)
	if err := app.Err(); err != nil {
		return fmt.Errorf("build application: %w", err)
	}
	if err := app.Start(ctx); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	logger.Zap().Info("session started", zap.Stringer("left", c.Left), zap.Stringer("right", c.Right))

	shown := false
	err = show(ctx, changesets, ports, panics, c, file.UI, stdout, func(c change.Comparison) {
		shown = true
		releases.Check()
		if watch {
			watches.Watch(c)
		}
	})
	if shown && err == nil {
		tellNewerRelease(ctx, releases, thisBuild().release(), stderr)
	}
	// Before the graph stops: the profile is to show what the session
	// holds, not what is left of it.
	err = errors.Join(err, writeHeapProfile(cfg.HeapProfile))
	return errors.Join(err, stopGraph(app))
}

// notesFiles are where the review notes of the worktree at root lie, in
// git's directory so that git status stays clean and each linked worktree
// has its own, with git's HEAD of that worktree beside them; empty without
// a repository, and then there are none.
func notesFiles(ctx context.Context, root string, logger *observability.Logger) usecase.NotesSettings {
	if root == "" {
		return usecase.NotesSettings{}
	}
	dir, err := git.FindGitDir(ctx, root)
	if err != nil {
		logger.Zap().Warn("no git directory: no notes", zap.Error(err))
		return usecase.NotesSettings{}
	}
	return usecase.NotesSettings{File: filepath.Join(dir, "seam", "notes.json"), Head: filepath.Join(dir, "HEAD")}
}

// stopGraph stops app within stopBudget. The deadline is fresh rather than
// derived from the session's context: on a signal that is already done, and
// the hooks would get no time at all. fx.StopTimeout does not apply to a
// manual Stop, so this deadline is the only one.
func stopGraph(app *fx.App) error {
	ctx, cancel := context.WithTimeout(context.Background(), stopBudget)
	defer cancel()
	if err := app.Stop(ctx); err != nil {
		return fmt.Errorf("stop: %w", err)
	}
	return nil
}

// show runs the TUI on the changeset of c after calling started with its
// resolved comparison, or lists its files when stdout is no terminal: a
// screen written into a pipe would be of no use to anyone.
func show(ctx context.Context, changesets in.ChangesetLoader, ports tui.Ports, panics supervise.PanicFunc, c change.Comparison, set ui.Settings, stdout io.Writer, started func(change.Comparison)) error {
	cs, err := changesets.LoadChangeset(ctx, c)
	if err != nil {
		return err
	}
	if !isTerminal(stdout) {
		return printChangeset(stdout, cs)
	}
	started(cs.Comparison)
	return tui.Run(ctx, cs, set, ports, panics, tui.IO{})
}

// tellNewerRelease writes the notice of a newer release once the TUI has
// given the terminal back. It is an extra: failing to write it, as to a
// closed stderr, does not fail a session that went well.
func tellNewerRelease(ctx context.Context, releases in.ReleaseNotifier, running release.Version, stderr io.Writer) {
	if latest, ok := releases.Newer(ctx); ok {
		_, _ = fmt.Fprintln(stderr, release.Notice(running, latest, updateCommand()))
	}
}

// isTerminal asks the device itself: a character device such as /dev/null
// is no terminal either.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(f.Fd())
}
