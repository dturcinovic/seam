package command

import (
	"context"
	"io"
	"os/exec"
	"time"
)

// stopWait is how long a command has after seam's context ends before it
// is killed: brew moving a cask must not be cut off midway.
const stopWait = 30 * time.Second

// Config is where a command reads and writes: the terminal of seam
// upgrade, which only cmd may hand on.
type Config struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// Runner implements out.CommandRunner.
type Runner struct {
	cfg Config
}

// New returns a runner over c.
func New(c Config) *Runner { return &Runner{cfg: c} }

// Run implements out.CommandRunner.
func (r *Runner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = r.cfg.Stdin, r.cfg.Stdout, r.cfg.Stderr
	// Ctrl-C ends seam's context, and the terminal interrupts the command
	// itself, in seam's process group. A second interrupt from seam would
	// cut short the cleanup the first one started, so seam sends none and
	// kills only a command that has not ended after stopWait.
	cmd.Cancel = func() error { return nil }
	cmd.WaitDelay = stopWait
	err := cmd.Run()
	// A command that ended well after seam's context did has done its work;
	// exec reports the context's end all the same.
	if err != nil && cmd.ProcessState != nil && cmd.ProcessState.Success() {
		return nil
	}
	// exec's errors name the command that is missing, and the caller names
	// the one that failed.
	return err
}
