package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"

	"seam/internal/adapter/driven/git"
	"seam/internal/app/port/in"
	"seam/internal/app/usecase"
	"seam/internal/config"
	"seam/internal/domain/change"
	"seam/internal/domain/limits"
	"seam/internal/observability"
)

// stopBudget bounds the whole shutdown, all OnStop hooks together.
const stopBudget = 3 * time.Second

// options is the graph of one session. It is the only place that knows use
// cases and adapters at once.
func options(cfg config.Config, logger *observability.Logger, root string) []fx.Option {
	return []fx.Option{
		// The default until the TUI's command line has --max-file-size.
		fx.Supply(cfg, logger, git.Config{Root: root}, limits.DefaultFileSize),
		observability.Module,
		git.Module,
		usecase.Module,
		// fx.WithLogger insists on the interface type; NewFxLogger returns
		// the concrete one.
		fx.WithLogger(func(l *observability.Logger) fxevent.Logger { return observability.NewFxLogger(l) }),
	}
}

// session finds the repository, builds the graph, loads the changeset of c,
// lists it on stdout and stops the graph within stopBudget. It owns the
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

	var changesets in.ChangesetLoader
	app := fx.New(append(options(cfg, logger, root), fx.Populate(&changesets))...)
	if err := app.Err(); err != nil {
		return fmt.Errorf("build application: %w", err)
	}
	if err := app.Start(ctx); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	logger.Zap().Info("session started", zap.Stringer("left", c.Left), zap.Stringer("right", c.Right))

	err = loadAndList(ctx, changesets, c, stdout)

	// Fresh rather than derived from ctx: on a signal ctx is already done,
	// and the hooks would get no time at all. fx.StopTimeout does not apply
	// to a manual Stop, so this deadline is the only one.
	stopCtx, cancel := context.WithTimeout(context.Background(), stopBudget)
	defer cancel()
	if stopErr := app.Stop(stopCtx); stopErr != nil {
		err = errors.Join(err, fmt.Errorf("stop: %w", stopErr))
	}
	return err
}

func loadAndList(ctx context.Context, changesets in.ChangesetLoader, c change.Comparison, stdout io.Writer) error {
	cs, err := changesets.LoadChangeset(ctx, c)
	if err != nil {
		return err
	}
	return printChangeset(stdout, cs)
}
