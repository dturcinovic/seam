package statefile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/dturcinovic/seam/internal/domain/release"
)

// Store keeps the last release check in one JSON file.
type Store struct {
	path string
}

// New returns the store of c; its file need not exist yet.
func New(c Config) *Store {
	return &Store{path: c.ReleaseFile}
}

// releaseCheck is the file as written.
type releaseCheck struct {
	Latest    string    `json:"latest,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// LoadCheck implements out.ReleaseStore.
func (s *Store) LoadCheck(ctx context.Context) (release.Check, error) {
	if err := ctx.Err(); err != nil {
		return release.Check{}, err
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return release.Check{}, nil
	}
	if err != nil {
		return release.Check{}, fmt.Errorf("read release check: %w", err)
	}
	var f releaseCheck
	if err := json.Unmarshal(data, &f); err != nil {
		return release.Check{}, fmt.Errorf("read release check %s: %w", s.path, err)
	}
	c := release.Check{At: f.CheckedAt}
	if f.Latest != "" {
		if c.Latest, err = release.ParseVersion(f.Latest); err != nil {
			return release.Check{}, fmt.Errorf("read release check %s: %w", s.path, err)
		}
	}
	return c, nil
}

// SaveCheck implements out.ReleaseStore. It writes a temporary file and
// renames it, so a session reading at the same time sees the old check or
// the new one, never half of one.
func (s *Store) SaveCheck(ctx context.Context, c release.Check) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(releaseCheck{Latest: c.Latest.String(), CheckedAt: c.At})
	if err != nil {
		return fmt.Errorf("encode release check: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("save release check: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), filepath.Base(s.path)+".*")
	if err != nil {
		return fmt.Errorf("save release check: %w", err)
	}
	// Once renamed, removing the old name fails harmlessly.
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close() // the write error is the one to report
		return fmt.Errorf("save release check: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("save release check: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("save release check: %w", err)
	}
	return nil
}
