package fswatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"

	"github.com/dturcinovic/seam/internal/app/port/out"
)

// NoteWatcher implements out.NoteWatcher.
type NoteWatcher struct {
	log *zap.Logger
}

// NewNoteWatcher returns a watcher that logs why watching stopped.
func NewNoteWatcher(log *zap.Logger) *NoteWatcher {
	return &NoteWatcher{log: log}
}

// WatchNotes implements out.NoteWatcher. It watches the file's directory,
// making it if need be: the file is replaced by renaming, which would end
// a watch on the file itself.
func (w *NoteWatcher) WatchNotes(ctx context.Context, file string, l out.NoteListener) error {
	err := watchNotes(ctx, file, l)
	if err != nil {
		w.log.Warn("notes watcher stopped", zap.Error(err))
	}
	return err
}

func watchNotes(ctx context.Context, file string, l out.NoteListener) error {
	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("watch notes: %w", err)
	}
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("watch notes: %w", limitHint(err))
	}
	// Closing ends the watcher's own goroutine; what it reports then is of
	// no use to anyone.
	defer func() { _ = fw.Close() }()
	if err := fw.Add(dir); err != nil {
		return fmt.Errorf("watch notes: %w", limitHint(err))
	}
	// What changed before the watch was set up cannot be told.
	l.Changed()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-fw.Events:
			if ev.Name == file {
				l.Changed()
			}
		case err := <-fw.Errors:
			// A full queue lost events, not the watch: a reload sees
			// whatever they were.
			if !errors.Is(err, fsnotify.ErrEventOverflow) {
				return fmt.Errorf("watch notes: %w", err)
			}
			l.Changed()
		}
	}
}
