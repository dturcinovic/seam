package lsp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/dturcinovic/seam/internal/domain/lang"
)

// macOS has no Pdeathsig. A server therefore runs under a guard, seam
// started again in the guard's role: it leads the server's process group
// and kills the group once the pipe from seam reaches EOF, which happens
// when seam ends, killed or not.

// guardEnv marks a process started as a server's guard.
const guardEnv = "SEAM_LSP_GUARD"

// guardFd is the guard's end of the pipe from seam: the first of cmd's
// ExtraFiles, after stdin, stdout and stderr.
const guardFd = 3

// serverCommand runs s under a guard, which leads a process group of its
// own. release closes seam's end of the guard's pipe; the guard then kills
// the group, should the server still run.
func serverCommand(s lang.Server) (cmd *exec.Cmd, release func() error, err error) {
	self, err := os.Executable()
	if err != nil {
		return nil, nil, fmt.Errorf("guard: %w", err)
	}
	fromSeam, seamEnd, err := os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("guard pipe: %w", err)
	}
	// Not tied to a request's context: the server serves the session, and
	// the pool ends it.
	cmd = exec.CommandContext(context.Background(), self, append([]string{s.Command}, s.Args...)...)
	cmd.Env = append(os.Environ(), guardEnv+"=1")
	cmd.ExtraFiles = []*os.File{fromSeam}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd, seamEnd.Close, nil
}

// ServeGuard serves as a server's guard if seam was started as one, and
// then reports the server's exit code for main to exit with. main calls it
// first, as does the TestMain of every package whose tests start servers:
// the guard is the running binary itself.
func ServeGuard() (code int, ok bool) {
	// A guard always names its server; without one, the mark leaked.
	if os.Getenv(guardEnv) == "" || len(os.Args) < 2 {
		return 0, false
	}
	return guard(os.Args[1:]), true
}

// guard runs the server args name, stdio and directory its own, and stays
// until the server ends or seam does.
func guard(args []string) int {
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool { return strings.HasPrefix(kv, guardEnv+"=") })
	// The pool's terminate signals the whole group. Only the server is to
	// end by it; the guard stays to kill what it leaves. Caught from before
	// the fork, so no SIGTERM finds the guard unprepared, and caught rather
	// than ignored, as exec resets a caught signal for the server but would
	// pass an ignored one on. Nothing reads the channel.
	signal.Notify(make(chan os.Signal, 1), syscall.SIGTERM)
	// Its own stdin, stdout and stderr are seam's pipes and the server log;
	// the server gets them unread, and not the pipe from seam.
	syscall.CloseOnExec(guardFd)
	pid, err := syscall.ForkExec(args[0], args, &syscall.ProcAttr{Env: env, Files: []uintptr{0, 1, 2}})
	if err != nil {
		// 127 as a shell has it: the command could not be run. The server
		// log is the guard's stderr, but the terminal rule keeps the guard
		// from writing there; seam logs the exit.
		return 127
	}
	if !serverEnded(pid) {
		// seam is gone, or the guard cannot tell: end the group, the guard
		// with it.
		_ = syscall.Kill(0, syscall.SIGKILL)
		return 1
	}
	return exitCode(pid)
}

// serverEnded blocks until the server pid ends, then reports true, or until
// the pipe from seam reaches EOF or kqueue fails, then false.
func serverEnded(pid int) bool {
	kq, err := syscall.Kqueue()
	if err != nil {
		return false
	}
	// Only the guard holds it, and it ends right after.
	defer func() { _ = syscall.Close(kq) }()
	changes := []syscall.Kevent_t{
		{Ident: uint64(pid), Filter: syscall.EVFILT_PROC, Flags: syscall.EV_ADD, Fflags: syscall.NOTE_EXIT},
		{Ident: guardFd, Filter: syscall.EVFILT_READ, Flags: syscall.EV_ADD},
	}
	events := make([]syscall.Kevent_t, len(changes))
	for {
		n, err := syscall.Kevent(kq, changes, events, nil)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			return false
		}
		// seam never writes to the pipe: any event on it is its EOF, and
		// counts even when the server ended in the same batch, as what the
		// server started may still run. A server that ended before it could
		// be watched is reported as an error of its event, which says the
		// same as its end.
		ended := false
		for _, e := range events[:n] {
			if e.Filter == syscall.EVFILT_READ {
				return false
			}
			ended = true
		}
		return ended
	}
}

// exitCode reaps the ended server and returns its exit code, a signal as a
// shell reports it.
func exitCode(pid int) int {
	// FindProcess cannot fail on Unix.
	p, _ := os.FindProcess(pid)
	state, err := p.Wait()
	if err != nil {
		return 1
	}
	if ws, ok := state.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return state.ExitCode()
}
