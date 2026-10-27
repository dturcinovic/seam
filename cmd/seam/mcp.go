package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/dturcinovic/seam/internal/adapter/driven/fswatch"
	"github.com/dturcinovic/seam/internal/adapter/driven/git"
	"github.com/dturcinovic/seam/internal/adapter/driven/notestore"
	"github.com/dturcinovic/seam/internal/adapter/driven/process"
	"github.com/dturcinovic/seam/internal/adapter/driving/mcp"
	"github.com/dturcinovic/seam/internal/app/usecase"
	"github.com/dturcinovic/seam/internal/config"
	"github.com/dturcinovic/seam/internal/observability"
)

func newMCPCommand() *cobra.Command {
	var flags config.Flags
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Serve the review notes of this worktree to an agent session",
		Long: `seam mcp is an MCP server on stdin and stdout, for an agent such as Claude
Code or OpenCode to start: the agent takes the notes set in seam and answers
them, and seam shows the answers. It runs in the worktree the agent works in.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(flags)
			if err != nil {
				return err
			}
			return mcpSession(cmd.Context(), cfg, cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVarP(&flags.Verbose, "verbose", "v", false, "log at debug level")
	return cmd
}

// mcpSession serves the notes of the worktree cfg.Dir is in until the
// agent closes stdin or a signal ends ctx. Nothing but the protocol goes
// to stdout; the log goes where a session's does.
func mcpSession(ctx context.Context, cfg config.Config, stdin io.Reader, stdout io.Writer) (err error) {
	root, err := git.FindRoot(ctx, cfg.Dir)
	if err != nil {
		return err
	}
	logger, err := observability.NewLogger(cfg)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, logger.Close()) }()
	notes := notesFile(ctx, root, logger)
	if notes == "" {
		return fmt.Errorf("no git directory for %s: no notes", root)
	}

	var ports mcp.Ports
	app := fx.New(
		fx.Supply(logger, notestore.Config{File: notes}, usecase.NotesSettings{File: notes}, usecase.AgentSettings{PID: os.Getpid()}),
		observability.Module,
		notestore.Module,
		process.Module,
		fswatch.Module,
		usecase.Module,
		fx.Populate(&ports.Sessions, &ports.Answerer, &ports.Taker, &ports.Batches),
		fxLogger(),
	)
	if err := app.Err(); err != nil {
		return fmt.Errorf("build application: %w", err)
	}
	if err := app.Start(ctx); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	err = mcp.ServeStdio(ctx, ports, io.NopCloser(stdin), nopCloser{stdout}, thisBuild().String(), logger.Zap())
	return errors.Join(err, stopGraph(app))
}

// nopCloser leaves stdout open: it is the process's, not the server's.
type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }
