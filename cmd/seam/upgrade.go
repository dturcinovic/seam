package main

import (
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dturcinovic/seam/internal/app/usecase"
	"github.com/dturcinovic/seam/internal/config"
	"github.com/dturcinovic/seam/internal/domain/release"
)

func newUpgradeCommand() *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade seam to the latest release",
		Long: `seam upgrade brings seam to the latest release the way it was installed:
it runs brew upgrade or go install, replaces the binary of a release archive
once its checksum matches, and names the commands for a .deb or .rpm, whose
package manager owns the binary. --check only tells what it would do.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(config.Flags{})
			if err != nil {
				return err
			}
			install, binary, module := thisInstall()
			set := usecase.UpgradeSettings{
				Running: thisBuild().release(), Install: install, Binary: binary, Module: module,
				GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
			}
			u := upgrades(cfg, set, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
			up, err := u.Upgrade(cmd.Context(), check)
			if up.Outcome == 0 {
				return err
			}
			// A failed write leaves the outcome to the exit code.
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), upgradeText(up))
			if coder, ok := errors.AsType[exitCoder](err); ok && coder.ExitCode() > 0 {
				return &exitCodeError{err: err, code: coder.ExitCode()}
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "only tell what seam upgrade would do")
	return cmd
}

// exitCoder is a command that ran and failed, as out.CommandRunner
// reports it.
type exitCoder interface {
	error
	ExitCode() int
}

// exitCodeError ends seam with code: seam upgrade ends as the command it
// ran, such as brew, ended.
type exitCodeError struct {
	err  error
	code int
}

func (e *exitCodeError) Error() string { return e.err.Error() }
func (e *exitCodeError) Unwrap() error { return e.err }

// upgradeText says what seam upgrade did, or what is left to the user; the
// error that left it, if any, the caller reports.
func upgradeText(up release.Upgrade) string {
	out := release.Notice(up.Running, up.Latest, "")
	switch up.Outcome {
	case release.Current:
		if up.Running.Newer(up.Latest) {
			return fmt.Sprintf("seam %s is newer than the latest release, %s", up.Running, up.Latest)
		}
		return "seam " + up.Running.String() + " is the latest release"
	case release.Ran:
		return "seam upgrade ran " + up.Commands[0]
	case release.Replaced:
		return fmt.Sprintf("seam upgraded from %s to %s: %s", up.Running, up.Latest, up.Binary)
	case release.WouldRun:
		return out + "seam upgrade runs " + up.Commands[0]
	case release.WouldReplace:
		return out + "seam upgrade replaces " + up.Binary
	case release.ByPackage:
		return out + "its package manager owns seam; install the package of " + releasePage + ":\n  " + strings.Join(up.Commands, "\n  ")
	case release.ByHand:
		if len(up.Commands) > 0 {
			return out + "run again: " + up.Commands[0]
		}
	}
	return out + "upgrade by hand from " + releasePage
}
