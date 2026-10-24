package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"seam/internal/config"
	"seam/internal/observability"
)

// stopBudget bounds the whole shutdown, all OnStop hooks together.
const stopBudget = 3 * time.Second

// options is the graph of one session. It is the only place that knows use
// cases and adapters at once.
func options(cfg config.Config, logger *observability.Logger) []fx.Option {
	return []fx.Option{
		fx.Supply(cfg, logger),
		observability.Module,
		// fx.WithLogger insists on the interface type; NewFxLogger returns
		// the concrete one.
		fx.WithLogger(func(l *observability.Logger) fxevent.Logger { return observability.NewFxLogger(l) }),
	}
}

// session builds the graph, runs it and stops it within stopBudget. It owns
// the logger and closes it only after the graph has stopped.
func session(ctx context.Context, cfg config.Config) (err error) {
	logger, err := observability.NewLogger(cfg)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, logger.Close()) }()

	app := fx.New(options(cfg, logger)...)
	if err := app.Err(); err != nil {
		return fmt.Errorf("build application: %w", err)
	}
	if err := app.Start(ctx); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	logger.Zap().Info("session started")

	// Fresh rather than derived from ctx: on a signal ctx is already done,
	// and the hooks would get no time at all. fx.StopTimeout does not apply
	// to a manual Stop, so this deadline is the only one.
	stopCtx, cancel := context.WithTimeout(context.Background(), stopBudget)
	defer cancel()
	if err := app.Stop(stopCtx); err != nil {
		return fmt.Errorf("stop: %w", err)
	}
	return nil
}
