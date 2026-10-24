package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/charmbracelet/x/term"
	"go.uber.org/fx"
	"go.uber.org/zap"

	"seam/internal/adapter/driven/git"
	"seam/internal/adapter/driving/tui"
	"seam/internal/app/port/in"
	"seam/internal/config"
	"seam/internal/domain/change"
	"seam/internal/observability"
	"seam/internal/supervise"
)

// stopBudget bounds the whole shutdown, all OnStop hooks together.
const stopBudget = 3 * time.Second

// session finds the repository, builds the graph, loads the changeset of c,
// shows it and stops the graph within stopBudget. It owns the
// logger and closes it only after the graph has stopped.
func session(ctx context.Context, cfg config.Config, c change.Comparison, stdout io.Writer) (err error) {
	// Before the graph: it runs git, which ctx can cancel, and its errors
	// are the user's to fix rather than faults of the wiring.
	root, err := git.FindRoot(ctx, cfg.Dir)
	if err != nil {
		return err
	}
	logger, err := observability.NewLogger(cfg)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, logger.Close()) }()

	var (
		changesets in.ChangesetLoader
		ports      tui.Ports
		panics     supervise.PanicFunc
	)
	app := fx.New(append(options(cfg, logger, root), fx.Populate(&changesets, &ports.Files, &ports.Reload, &panics))...)
	if err := app.Err(); err != nil {
		return fmt.Errorf("build application: %w", err)
	}
	if err := app.Start(ctx); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	logger.Zap().Info("session started", zap.Stringer("left", c.Left), zap.Stringer("right", c.Right))

	err = show(ctx, changesets, ports, panics, c, stdout)
	return errors.Join(err, stopGraph(app))
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

// show runs the TUI on the changeset of c, or lists its files when stdout is
// no terminal: a screen written into a pipe would be of no use to anyone.
func show(ctx context.Context, changesets in.ChangesetLoader, ports tui.Ports, panics supervise.PanicFunc, c change.Comparison, stdout io.Writer) error {
	cs, err := changesets.LoadChangeset(ctx, c)
	if err != nil {
		return err
	}
	if !isTerminal(stdout) {
		return printChangeset(stdout, cs)
	}
	return tui.Run(ctx, cs, ports, panics, tui.IO{})
}

// isTerminal asks the device itself: a character device such as /dev/null
// is no terminal either.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(f.Fd())
}
