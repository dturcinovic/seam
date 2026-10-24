// Command seam shows the changes of a git repository side by side in the
// terminal.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/spf13/cobra"

	"seam/internal/config"
)

// version is stamped by `task build`. A plain `go build` leaves it empty, and
// the binary then reports the VCS revision instead of claiming a release.
var version = "" //nolint:gochecknoglobals // set through -ldflags -X, which needs a variable

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cmd := newRootCommand()
	cmd.SetArgs(args)
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
	var staged bool
	cmd := &cobra.Command{
		Use:     "seam [<rev> [<rev>]]",
		Short:   "Side-by-side diff viewer with type info from the language server",
		Version: versionString(),
		Long: `Without arguments seam compares HEAD with the worktree, untracked files
included. One revision replaces HEAD, two compare two commits, and --staged
compares HEAD with the index.`,
		Args: cobra.MaximumNArgs(2),
		// Errors are printed once by run; usage only on --help.
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := comparison(args, staged)
			if err != nil {
				return err
			}
			cfg, err := config.Load(flags)
			if err != nil {
				return err
			}
			return session(cmd.Context(), cfg, c, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVarP(&flags.Verbose, "verbose", "v", false, "log at debug level")
	cmd.Flags().BoolVar(&staged, "staged", false, "compare HEAD with the index")
	return cmd
}

func versionString() string {
	if version != "" {
		return version
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	commit, modified := "", false
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			commit = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	switch {
	case commit == "":
		return "dev"
	case modified:
		return commit + "-dirty"
	}
	return commit
}
