package files

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/dturcinovic/seam/internal/domain/change"
)

// compareChunk is how much of two files is compared at a time.
const compareChunk = 64 << 10

// ListPaths implements out.PathLister. Anything but a regular file, or a
// link to one, is left out: seam could not show it either.
func (d *Disk) ListPaths(ctx context.Context, left, right string) ([]change.ChangedFile, error) {
	old, err := regularFiles(ctx, left)
	if err != nil {
		return nil, err
	}
	new, err := regularFiles(ctx, right)
	if err != nil {
		return nil, err
	}
	var files []change.ChangedFile
	buf := &compareBuffers{}
	for rel := range old {
		if _, ok := new[rel]; !ok {
			files = append(files, change.ChangedFile{Status: change.Deleted, OldPath: rel})
		}
	}
	for rel, size := range new {
		oldSize, ok := old[rel]
		if !ok {
			files = append(files, change.ChangedFile{Status: change.Added, NewPath: rel})
			continue
		}
		same := oldSize == size
		if same {
			if same, err = buf.sameContent(ctx, filepath.Join(left, rel), filepath.Join(right, rel)); err != nil {
				return nil, err
			}
		}
		if !same {
			files = append(files, change.ChangedFile{Status: change.Modified, OldPath: rel, NewPath: rel})
		}
	}
	slices.SortFunc(files, func(a, b change.ChangedFile) int { return cmp.Compare(a.Path(), b.Path()) })
	return files, nil
}

// regularFiles maps the regular files below dir, by their slash-separated
// path relative to it, to their size.
func regularFiles(ctx context.Context, dir string) (map[string]int64, error) {
	// WalkDir does not enter a root that is a link to a directory.
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}
	files := map[string]int64{}
	err = filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("list %s: %w", dir, err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		var info fs.FileInfo
		switch {
		case e.Type().IsRegular():
			info, err = e.Info()
		case e.Type()&fs.ModeSymlink != 0:
			info, err = os.Stat(path)
		default:
			return nil
		}
		if err != nil || !info.Mode().IsRegular() {
			// A dangling link or a special file has no content to compare.
			return nil //nolint:nilerr // left out, as the doc of ListPaths says
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("list %s: %w", dir, err)
		}
		files[filepath.ToSlash(rel)] = info.Size()
		return nil
	})
	return files, err
}

// compareBuffers are reused for every pair of files compared.
type compareBuffers struct {
	a, b [compareChunk]byte
}

// sameContent compares two files of the same size.
func (buf *compareBuffers) sameContent(ctx context.Context, a, b string) (bool, error) {
	fa, err := os.Open(a)
	if err != nil {
		return false, pathError(a, err)
	}
	defer func() { _ = fa.Close() }() // Read only: closing cannot lose data.
	fb, err := os.Open(b)
	if err != nil {
		return false, pathError(b, err)
	}
	defer func() { _ = fb.Close() }() // Read only: closing cannot lose data.
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		na, errA := io.ReadFull(fa, buf.a[:])
		nb, errB := io.ReadFull(fb, buf.b[:])
		if err := errors.Join(unended(a, errA), unended(b, errB)); err != nil {
			return false, err
		}
		if na != nb || !bytes.Equal(buf.a[:na], buf.b[:nb]) {
			return false, nil
		}
		if errA != nil || errB != nil {
			return ended(errA) && ended(errB), nil
		}
	}
}

// ended reports whether a full read reached the end of the file.
func ended(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// unended is a read error other than the end of the file.
func unended(path string, err error) error {
	if err == nil || ended(err) {
		return nil
	}
	return pathError(path, err)
}
