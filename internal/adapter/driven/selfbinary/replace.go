package selfbinary

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// Replacer implements out.BinaryReplacer.
type Replacer struct{}

// New returns a replacer.
func New() *Replacer { return &Replacer{} }

// Replace implements out.BinaryReplacer. The new binary is written next to
// the old one and renamed over it: a rename within a directory is atomic,
// so the path never names half a binary, and a running seam keeps the old
// file it was started from. Its errors are os's, which name the file and
// what failed.
func (*Replacer) Replace(ctx context.Context, path string, data []byte) (err error) {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".upgrade-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.Remove(tmp.Name()))
		}
	}()
	if err := write(tmp, data, info.Mode().Perm()); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// write fills f with data and gives it mode, on disk before the rename, so
// that a crash leaves the old binary or the whole new one.
func write(f *os.File, data []byte, mode os.FileMode) error {
	_, err := f.Write(data)
	if err == nil {
		err = f.Chmod(mode)
	}
	if err == nil {
		err = f.Sync()
	}
	return errors.Join(err, f.Close())
}
