package testsupport

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Golden compares got with the golden file at path. With update, as a
// test's -update flag asks, it writes got there first.
func Golden(t *testing.T, path, got string, update bool) {
	t.Helper()
	if update {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "run with -update to create it")
	assert.Equal(t, string(want), got)
}
