package git

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/limits"
)

// A /proc file reports size 0 to stat and has content all the same: the
// same as a file that grows after Stat, and deterministic.
func TestFileThatGrowsAfterStatIsCaughtByTheRead(t *testing.T) {
	root, err := os.OpenRoot("/proc/self")
	require.NoError(t, err)
	defer func() { _ = root.Close() }()
	info, err := root.Lstat("status")
	require.NoError(t, err)
	require.Zero(t, info.Size())

	max, err := limits.NewFileSize(10)
	require.NoError(t, err)
	_, _, err = readLimited(root, "status", max)

	tooLarge, ok := errors.AsType[*change.TooLargeError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, int64(11), tooLarge.Size, "reading stops one byte past the limit")
}
