package lsp

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/dturcinovic/seam/internal/domain/lang"
)

// errMissing is a server whose program is not installed.
var errMissing = errors.New("language server not installed")

// conn is a started server: the pipes to and from it, and its end. The
// caller of wait must be the goroutine that called start, which has locked
// its thread: see processes.start.
type conn struct {
	in  io.WriteCloser
	out io.ReadCloser
	// terminate asks every process of the server's group to end; kill ends
	// them. No other code knows signals.
	terminate func()
	kill      func()
	// wait blocks until the server process has ended.
	wait func() error
	pid  int
}

// starter finds and starts servers.
type starter interface {
	// find resolves a server's command to the program to run, or fails
	// with errMissing.
	find(command string) (string, error)
	// start runs s, whose command find has resolved, in dir.
	start(s lang.Server, dir string) (*conn, error)
}

// processes starts servers as child processes, each in a process group of
// its own, so that ending the group also ends what the server started, as
// tsserver under typescript-language-server, which holds the pipes too.
type processes struct {
	logDir string

	mu sync.Mutex
	// logged are the servers whose stderr file this session has started.
	logged map[string]bool
}

func newProcesses(logDir string) *processes {
	return &processes{logDir: logDir, logged: make(map[string]bool)}
}

func (p *processes) find(command string) (string, error) {
	path, err := exec.LookPath(command)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", errMissing, command, err)
	}
	return path, nil
}

// start runs s in dir. Its caller must have locked its OS thread and must
// stay on it until wait returns: on Linux, Pdeathsig ends the server when
// the thread that started it ends, which would otherwise be any thread Go
// retires.
func (p *processes) start(s lang.Server, dir string) (*conn, error) {
	stderr, err := p.stderr(s.Name)
	if err != nil {
		return nil, err
	}
	toServer, in, err := os.Pipe()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("pipe: %w", err), stderr.Close())
	}
	out, fromServer, err := os.Pipe()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("pipe: %w", err), toServer.Close(), in.Close(), stderr.Close())
	}
	cmd, release, err := serverCommand(s)
	if err != nil {
		return nil, errors.Join(err, toServer.Close(), fromServer.Close(), stderr.Close(), in.Close(), out.Close())
	}
	cmd.Dir = dir
	// Files rather than StdinPipe and StdoutPipe: Wait closes those, which
	// would race the reader still reading what the server wrote last.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = toServer, fromServer, stderr
	err = cmd.Start()
	// The child has its own copies now, of the extra files too; with these
	// closed, EOF comes when the last process holding the pipe ends. The
	// kernel frees a descriptor even when close reports an error, so there
	// is nothing to act on.
	_ = errors.Join(toServer.Close(), fromServer.Close(), stderr.Close())
	for _, f := range cmd.ExtraFiles {
		_ = f.Close()
	}
	if err != nil {
		return nil, errors.Join(fmt.Errorf("start %s: %w", s.Command, err), in.Close(), out.Close(), release())
	}
	pid := cmd.Process.Pid
	// The group may be gone already, which is what the signals are for.
	return &conn{
		in: in, out: out, pid: pid,
		terminate: func() { _ = syscall.Kill(-pid, syscall.SIGTERM) },
		kill:      func() { _ = syscall.Kill(-pid, syscall.SIGKILL) },
		wait: func() error {
			err := cmd.Wait()
			// Freed like the pipes above, whatever close reports.
			_ = release()
			return err
		},
	}, nil
}

// stderr opens the server's stderr file: anew for its first start in a
// session, appended to after that, so instances of one server share it.
func (p *processes) stderr(name string) (*os.File, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	flags := os.O_WRONLY | os.O_CREATE | os.O_APPEND
	if !p.logged[name] {
		flags |= os.O_TRUNC
	}
	if err := os.MkdirAll(p.logDir, 0o700); err != nil {
		return nil, fmt.Errorf("server log directory: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(p.logDir, "lsp-"+name+".log"), flags, 0o600)
	if err != nil {
		return nil, fmt.Errorf("server log: %w", err)
	}
	p.logged[name] = true
	return f, nil
}
