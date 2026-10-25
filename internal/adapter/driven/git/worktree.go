package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"syscall"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/limits"
)

// ReadWorktree implements out.WorktreeReader. Paths come from git and the
// user, so every access goes through os.Root and stays inside the worktree.
func (r *Repo) ReadWorktree(ctx context.Context, path string, max limits.FileSize) ([]byte, change.Stamp, error) {
	if err := ctx.Err(); err != nil {
		return nil, change.Stamp{}, err
	}
	data, info, err := readLimited(r.files, path, max)
	if err != nil {
		return nil, change.Stamp{}, err
	}
	return data, stamp(info), nil
}

// StatWorktree implements out.WorktreeStatter.
func (r *Repo) StatWorktree(ctx context.Context, path string) (change.Stamp, error) {
	if err := ctx.Err(); err != nil {
		return change.Stamp{}, err
	}
	info, err := r.files.Lstat(path)
	if err != nil {
		return change.Stamp{}, worktreeError(path, err)
	}
	return stamp(info), nil
}

// readLimited reads a regular file or a symlink's target. The kind is decided
// by Lstat, before anything is opened: opening a device can have effects of
// its own. The open descriptor is checked again in case the path was
// swapped meanwhile.
func readLimited(root *os.Root, path string, max limits.FileSize) ([]byte, fs.FileInfo, error) {
	info, err := root.Lstat(path)
	switch {
	case err != nil:
		return nil, nil, worktreeError(path, err)
	case info.Mode()&fs.ModeSymlink != 0:
		return readLink(root, path, info, max)
	case !info.Mode().IsRegular():
		return nil, nil, fmt.Errorf("%s: %w", path, change.ErrNotRegular)
	}
	// O_NONBLOCK keeps a FIFO swapped in since Lstat from blocking the open.
	f, err := root.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, worktreeError(path, err)
	}
	defer func() { _ = f.Close() }() // Read only: closing cannot lose data.
	if info, err = f.Stat(); err != nil {
		return nil, nil, worktreeError(path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%s: %w", path, change.ErrNotRegular)
	}
	if info.Size() > max.Bytes() {
		return nil, nil, &change.TooLargeError{Size: info.Size()}
	}
	// Sized from Stat so a file of up to 8 MiB is one allocation; the file
	// may still grow before the read, and the bytes read decide.
	var buf bytes.Buffer
	buf.Grow(int(info.Size()) + bytes.MinRead)
	if _, err := buf.ReadFrom(io.LimitReader(f, max.Bytes()+1)); err != nil {
		return nil, nil, worktreeError(path, err)
	}
	if int64(buf.Len()) > max.Bytes() {
		return nil, nil, &change.TooLargeError{Size: int64(buf.Len())}
	}
	return buf.Bytes(), info, nil
}

// readLink returns the link's target as its content, as git stores it.
func readLink(root *os.Root, path string, info fs.FileInfo, max limits.FileSize) ([]byte, fs.FileInfo, error) {
	target, err := root.Readlink(path)
	if err != nil {
		return nil, nil, worktreeError(path, err)
	}
	if int64(len(target)) > max.Bytes() {
		return nil, nil, &change.TooLargeError{Size: int64(len(target))}
	}
	return []byte(target), info, nil
}

func worktreeError(path string, err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%s: %w", path, change.ErrNotExist)
	}
	return fmt.Errorf("read %s: %w", path, err)
}

func stamp(info fs.FileInfo) change.Stamp {
	s := change.Stamp{Size: info.Size(), ModTime: info.ModTime()}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		s.Inode = st.Ino
	}
	return s
}
