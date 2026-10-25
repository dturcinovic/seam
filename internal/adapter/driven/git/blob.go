package git

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/limits"
	"github.com/dturcinovic/seam/internal/supervise"
)

// exitGrace is how long cat-file may take to exit after its input closes.
const exitGrace = 100 * time.Millisecond

// catFile is a running `git cat-file --batch-command`. It takes object ids
// only, never paths, so no file name can break its line protocol. Its
// `info` command gives the size before `contents` sends any byte, which
// keeps an oversized blob off the pipe.
type catFile struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	out *bufio.Reader
	// pipe is out's read end, which abort closes.
	pipe io.Closer
}

// ReadBlob implements out.BlobReader.
func (r *Repo) ReadBlob(ctx context.Context, id change.ObjectID, max limits.FileSize) ([]byte, error) {
	// git's answer to an empty id has a shape of its own; the zero id is
	// the missing side of a file, so it never reaches the process.
	if id.IsZero() {
		return nil, fmt.Errorf("blob of a missing side: %w", change.ErrNotExist)
	}
	r.blobMu.Lock()
	defer r.blobMu.Unlock()
	if r.closed {
		return nil, ErrClosed
	}
	// Under the lock: a ctx that ended while it waited would make the read
	// kill a cat-file that is in step.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.blobs == nil {
		cat, err := r.startCatFile()
		if err != nil {
			return nil, err
		}
		r.blobs = cat
	}
	data, err := r.readAbortable(ctx, r.blobs, id, max)
	if errors.Is(err, errProtocol) {
		// The process is in an unknown state; the next read starts afresh.
		r.blobs.kill()
		r.blobs = nil
	}
	return data, err
}

// readAbortable reads id from cat. In a partial clone cat-file fetches a
// missing blob from the remote, which can take any time; if ctx ends or
// Close stops r.reads first, the read is aborted. A process aborted is out
// of step whatever the read returned.
func (r *Repo) readAbortable(ctx context.Context, cat *catFile, id change.ObjectID, max limits.FileSize) ([]byte, error) {
	done := make(chan struct{})
	// killed carries why the read was aborted, nil if it was not.
	killed := make(chan error, 1)
	err := r.reads.Go("abort cat-file read", func(stop context.Context) {
		var cause error
		select {
		case <-done:
			killed <- nil
			return
		case <-ctx.Done():
			cause = ctx.Err()
		case <-stop.Done():
			cause = ErrClosed
		}
		cat.abort()
		killed <- cause
	})
	if errors.Is(err, supervise.ErrStopped) {
		err = ErrClosed
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errProtocol, err)
	}
	data, err := cat.read(id, max)
	close(done)
	if cause := <-killed; cause != nil {
		return nil, fmt.Errorf("%w: read %s: %w", errProtocol, id, cause)
	}
	return data, err
}

func (r *Repo) startCatFile() (*catFile, error) {
	// Not tied to a request's context: the process serves the whole session
	// and Close ends it.
	cmd := command(context.Background(), r.root, "cat-file", "--batch-command")
	// A group of its own, so that killing it ends a lazy fetch it started.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("cat-file: %w", err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("cat-file: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start cat-file: %w", err)
	}
	return &catFile{cmd: cmd, in: in, out: bufio.NewReader(out), pipe: out}, nil
}

// errProtocol marks a failure that leaves the process out of step.
var errProtocol = errors.New("cat-file protocol")

func (c *catFile) read(id change.ObjectID, max limits.FileSize) ([]byte, error) {
	kind, size, err := c.request("info", id)
	if err != nil {
		return nil, err
	}
	switch {
	case kind == "missing":
		return nil, fmt.Errorf("blob %s: %w", id, change.ErrNotExist)
	case kind != "blob":
		return nil, fmt.Errorf("object %s is a %s: %w", id, kind, change.ErrNotRegular)
	case size > max.Bytes():
		return nil, &change.TooLargeError{Size: size}
	}
	if _, _, err := c.request("contents", id); err != nil {
		return nil, err
	}
	// The content is followed by a newline of the protocol's own.
	data := make([]byte, size+1)
	if _, err := io.ReadFull(c.out, data); err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", errProtocol, id, err)
	}
	return data[:size], nil
}

// request sends one command and reads its header line: "<id> <type> <size>"
// or "<id> missing".
func (c *catFile) request(command string, id change.ObjectID) (kind string, size int64, err error) {
	if _, err := fmt.Fprintf(c.in, "%s %s\n", command, id); err != nil {
		return "", 0, fmt.Errorf("%w: write: %w", errProtocol, err)
	}
	line, err := c.out.ReadString('\n')
	if err != nil {
		return "", 0, fmt.Errorf("%w: read: %w", errProtocol, err)
	}
	fields := strings.Fields(line)
	switch {
	case len(fields) == 2 && fields[1] == "missing":
		return "missing", 0, nil
	case len(fields) != 3 || fields[0] != id.String():
		return "", 0, fmt.Errorf("%w: unexpected answer %q", errProtocol, line)
	}
	size, err = strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("%w: size in %q: %w", errProtocol, line, err)
	}
	return fields[1], size, nil
}

// close ends the process: closed input makes cat-file exit, and after
// exitGrace it is killed.
func (c *catFile) close(tasks *supervise.Group) error {
	_ = c.in.Close() // Wait reports what matters; the pipe is gone either way.
	timer, err := tasks.AfterFunc(exitGrace, "kill cat-file", c.killGroup)
	if err != nil {
		c.kill()
		return err
	}
	err = c.cmd.Wait()
	if !timer.Stop() {
		return nil // Killed on purpose, so its exit status says nothing.
	}
	return err
}

// kill ends a process whose state is unknown. Its errors say only that the
// process or pipe is gone already, and its exit status that it was killed.
func (c *catFile) kill() {
	_ = c.in.Close()
	c.killGroup()
	_ = c.cmd.Wait()
}

// abort ends a read in flight. A lazy fetch started by cat-file holds its
// stdout too, so the kill alone would leave the read waiting; closing the
// pipe ends it.
func (c *catFile) abort() {
	_ = c.pipe.Close() // a second close, by Wait, changes nothing
	c.killGroup()
}

// killGroup ends cat-file and what it started. It fails only if they have
// exited already, which is the aim.
func (c *catFile) killGroup() { _ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL) }
