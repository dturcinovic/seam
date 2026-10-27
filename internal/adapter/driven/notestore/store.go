package notestore

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/dturcinovic/seam/internal/domain/note"
)

// Waiting for the lock: tried every lockPoll until lockWait has passed.
const (
	lockPoll = 20 * time.Millisecond
	lockWait = 2 * time.Second
)

// ErrLocked is a lock another process held past the wait.
var ErrLocked = errors.New("notes file locked by another process")

// Store implements out.NoteStore over one file.
type Store struct {
	path string
}

// New returns the store of c; its file need not exist yet.
func New(c Config) *Store {
	return &Store{path: c.File}
}

// Load implements out.NoteStore. It takes no lock: the file is only ever
// replaced whole, so it is read either before a change or after.
func (s *Store) Load(ctx context.Context) (note.Book, error) {
	if err := ctx.Err(); err != nil {
		return note.Book{}, err
	}
	if s.path == "" {
		return note.Book{}, note.ErrNoBook
	}
	return s.read()
}

// Change implements out.NoteStore: it reads, changes and writes the book
// under the lock, so changes of other processes in between are neither
// lost nor lose this one.
func (s *Store) Change(ctx context.Context, change func(*note.Book) error) error {
	if s.path == "" {
		return note.ErrNoBook
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("change notes: %w", err)
	}
	unlock, err := lock(ctx, filepath.Join(dir, "notes.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	b, err := s.read()
	if err != nil {
		return err
	}
	if err := change(&b); err != nil {
		return err
	}
	data, err := encode(b)
	if err != nil {
		return err
	}
	if len(data) > note.MaxFile {
		return fmt.Errorf("%w: %d bytes, at most %d", note.ErrBookTooLarge, len(data), note.MaxFile)
	}
	return s.write(data)
}

// read reads the book; a missing file is an empty one.
func (s *Store) read() (note.Book, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return note.Book{}, nil
	}
	if err != nil {
		return note.Book{}, fmt.Errorf("read notes: %w", err)
	}
	b, err := decode(data)
	if err != nil {
		return note.Book{}, fmt.Errorf("read notes: %w", err)
	}
	return b, nil
}

// write writes a temporary file and renames it, so a reader sees the old
// book or the new one, never half of one.
func (s *Store) write(data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(s.path), filepath.Base(s.path)+".*")
	if err != nil {
		return fmt.Errorf("save notes: %w", err)
	}
	// Once renamed, removing the old name fails harmlessly.
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close() // the write error is the one to report
		return fmt.Errorf("save notes: %w", err)
	}
	// On disk before the rename: a crash must not leave a short file in
	// the book's place, which seam would then refuse to write over.
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close() // the sync error is the one to report
		return fmt.Errorf("save notes: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("save notes: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("save notes: %w", err)
	}
	return nil
}

// lock takes the lock file at path, trying without blocking until
// lockWait has passed or ctx ends: a blocking flock could not be given
// up.
func lock(ctx context.Context, path string) (unlock func(), err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock notes: %w", err)
	}
	deadline := time.Now().Add(lockWait)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			// Closing the file releases the lock.
			return func() { _ = f.Close() }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) || time.Now().After(deadline) {
			_ = f.Close() // the lock's error is the one to report
			if errors.Is(err, syscall.EWOULDBLOCK) {
				err = ErrLocked
			}
			return nil, fmt.Errorf("lock notes: %w", err)
		}
		select {
		case <-ctx.Done():
			_ = f.Close() // nothing was locked
			return nil, ctx.Err()
		case <-time.After(lockPoll):
		}
	}
}
