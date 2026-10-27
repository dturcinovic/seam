package files

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

// ReadPath implements out.PathReader. Unlike the worktree's files, these
// are not read through os.Root: the links of git difftool -d point out of
// their directory by design.
func (d *Disk) ReadPath(ctx context.Context, path string, max limits.FileSize) ([]byte, change.Stamp, error) {
	if err := ctx.Err(); err != nil {
		return nil, change.Stamp{}, err
	}
	// The kind is decided before anything is opened: opening a device can
	// have effects of its own.
	info, err := os.Stat(path)
	if err != nil {
		return nil, change.Stamp{}, pathError(path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, change.Stamp{}, fmt.Errorf("%s: %w", path, change.ErrNotRegular)
	}
	// O_NONBLOCK keeps a FIFO swapped in since Stat from blocking the open;
	// the descriptor is checked again for the same reason.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, change.Stamp{}, pathError(path, err)
	}
	defer func() { _ = f.Close() }() // Read only: closing cannot lose data.
	if info, err = f.Stat(); err != nil {
		return nil, change.Stamp{}, pathError(path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, change.Stamp{}, fmt.Errorf("%s: %w", path, change.ErrNotRegular)
	}
	if info.Size() > max.Bytes() {
		return nil, change.Stamp{}, &change.TooLargeError{Size: info.Size()}
	}
	var buf bytes.Buffer
	buf.Grow(int(info.Size()) + bytes.MinRead)
	if _, err := buf.ReadFrom(io.LimitReader(f, max.Bytes()+1)); err != nil {
		return nil, change.Stamp{}, pathError(path, err)
	}
	if int64(buf.Len()) > max.Bytes() {
		return nil, change.Stamp{}, &change.TooLargeError{Size: int64(buf.Len())}
	}
	return buf.Bytes(), stamp(info), nil
}

// StatPath implements out.PathReader.
func (d *Disk) StatPath(ctx context.Context, path string) (change.Stamp, error) {
	if err := ctx.Err(); err != nil {
		return change.Stamp{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return change.Stamp{}, pathError(path, err)
	}
	return stamp(info), nil
}

func stamp(info fs.FileInfo) change.Stamp {
	s := change.Stamp{Size: info.Size(), ModTime: info.ModTime()}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		s.Inode = st.Ino
	}
	return s
}

func pathError(path string, err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%s: %w", path, change.ErrNotExist)
	}
	return fmt.Errorf("read %s: %w", path, err)
}
