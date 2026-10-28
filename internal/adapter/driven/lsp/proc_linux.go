package lsp

import (
	"context"
	"os/exec"
	"syscall"

	"github.com/dturcinovic/seam/internal/domain/lang"
)

// serverCommand runs s directly, in a process group of its own. Pdeathsig
// ends it when seam is killed and cannot end it itself, so release has
// nothing to free.
func serverCommand(s lang.Server) (cmd *exec.Cmd, release func() error, err error) {
	// Not tied to a request's context: the server serves the session, and
	// the pool ends it.
	cmd = exec.CommandContext(context.Background(), s.Command, s.Args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	return cmd, func() error { return nil }, nil
}

// ServeGuard serves as a server's guard on macOS. Linux has Pdeathsig and
// starts no guard, so here it never serves.
func ServeGuard() (code int, ok bool) { return 0, false }
