// Command seam shows the changes of a git repository side by side in the
// terminal.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/dturcinovic/seam/internal/config"
	"github.com/dturcinovic/seam/internal/domain/change"
)

func main() {
	// A language server's guard is seam started again; it must not become
	// a second seam.
	if code, ok := serveGuard(); ok {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cmd := newRootCommand()
	cmd.SetArgs(args)
	cmd.SetIn(stdin)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	if err := cmd.ExecuteContext(ctx); err != nil {
		// A failed write of the last error has nowhere left to be reported.
		_, _ = fmt.Fprintln(stderr, "seam:", printableLines(err.Error()))
		return 1
	}
	return 0
}

func newRootCommand() *cobra.Command {
	var flags config.Flags
	var staged, watch, onDisk, showVersion bool
	cmd := &cobra.Command{
		Use:   "seam [<rev> [<rev>]] | seam --files <left> <right>",
		Short: "Side-by-side diff viewer with type info from the language server",
		Long: `Without arguments seam compares HEAD with the worktree, untracked files
included. One revision replaces HEAD, two compare two commits, and --staged
compares HEAD with the index. --watch reloads whenever the worktree, the
index or HEAD changes. --files compares two files or two directories on
disk, as git difftool hands them over.`,
		// --version takes no arguments into account, as cobra's own does not.
		Args: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				return nil
			}
			return cobra.MaximumNArgs(2)(cmd, args)
		},
		// Errors are printed once by run; usage only on --help.
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				return printVersion(cmd.Context(), flags, cmd.OutOrStdout())
			}
			c, err := comparison(args, staged, onDisk)
			if err != nil {
				return err
			}
			if watch {
				if _, err := change.WatchesWorktree(c); err != nil {
					return fmt.Errorf("--watch: %w", err)
				}
			}
			cfg, err := config.Load(flags)
			if err != nil {
				return err
			}
			return session(cmd.Context(), cfg, c, watch, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVarP(&flags.Verbose, "verbose", "v", false, "log at debug level")
	cmd.Flags().BoolVar(&staged, "staged", false, "compare HEAD with the index")
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "reload when the worktree, the index or HEAD changes")
	cmd.Flags().BoolVar(&onDisk, "files", false, "compare two files or two directories on disk, as for git difftool")
	cmd.Flags().StringVar(&flags.MaxFileSize, "max-file-size", "", "largest side of a file to load, in bytes or with K, M or G (default 8M)")
	cmd.Flags().StringVar(&flags.HeapProfile, "pprof", "", "write a heap profile to this file when seam ends")
	// Not cobra's own --version, which prints before the configuration is
	// known: the notice of a newer release needs it.
	cmd.Flags().BoolVar(&showVersion, "version", false, "print the version")
	cmd.AddCommand(newMCPCommand())
	// A subcommand takes a name a revision could have; only mcp is worth
	// that, not cobra's shell completion.
	cmd.CompletionOptions.DisableDefaultCmd = true
	return cmd
}

// printVersion prints the version and the notice of a newer release, if the
// last check found one; as in a session, failing to write the notice fails
// nothing.
func printVersion(ctx context.Context, flags config.Flags, stdout io.Writer) error {
	b := thisBuild()
	if _, err := fmt.Fprintln(stdout, "seam version "+b.String()); err != nil {
		return err
	}
	if line, ok := releaseNotice(ctx, flags, b.release()); ok {
		_, _ = fmt.Fprintln(stdout, line)
	}
	return nil
}
