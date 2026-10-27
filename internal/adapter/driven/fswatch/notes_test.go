package fswatch_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/dturcinovic/seam/internal/adapter/driven/fswatch"
	"github.com/dturcinovic/seam/internal/testsupport"
)

// watchNotes watches notes.json in a directory not made yet, until the
// test ends; it returns once the watch reported it is set up.
func watchNotes(t *testing.T) (watched, string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "git", "seam", "notes.json")
	w := watched{listener: newListener()}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- fswatch.NewNoteWatcher(zap.NewNop()).WatchNotes(ctx, file, w.listener)
	}()
	t.Cleanup(func() {
		cancel()
		assert.NoError(t, <-done)
	})
	select {
	case <-w.changes:
	case <-time.After(testsupport.Guard):
		t.Fatal("the watcher did not report it is set up")
	}
	return w, file
}

func TestTheNotesFileIsWatchedThroughRenames(t *testing.T) {
	w, file := watchNotes(t)
	dir := filepath.Dir(file)

	assert.True(t, w.changesOn(t, func() {
		tmp := filepath.Join(dir, "notes.json.123")
		require.NoError(t, os.WriteFile(tmp, []byte("{}"), 0o600))
		require.NoError(t, os.Rename(tmp, file))
	}), "a file renamed into place")
	assert.True(t, w.changesOn(t, func() {
		tmp := filepath.Join(dir, "notes.json.456")
		require.NoError(t, os.WriteFile(tmp, []byte("{}"), 0o600))
		require.NoError(t, os.Rename(tmp, file))
	}), "and again, once the first is replaced")
	assert.False(t, w.changesOn(t, func() {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.lock"), nil, 0o600))
	}), "the lock is no change")
}

func TestAMissingNotesDirectoryFailsIfItCannotBeMade(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))
	err := fswatch.NewNoteWatcher(zap.NewNop()).WatchNotes(t.Context(), filepath.Join(blocker, "seam", "notes.json"), newListener())
	require.Error(t, err)
}
