package selfbinary_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/adapter/driven/selfbinary"
)

func TestTheBinaryIsReplacedWithItsMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seam")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o751))

	require.NoError(t, selfbinary.New().Replace(t.Context(), path, []byte("new")))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new", string(data))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o751), info.Mode().Perm())
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temporary file is left")
}

func TestABinaryNotReplacedStaysAsItWas(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "seam")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o755))
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) }) // so that TempDir can remove it

	err := selfbinary.New().Replace(t.Context(), path, []byte("new"))
	require.ErrorIs(t, err, os.ErrPermission)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "old", string(data))

	require.Error(t, selfbinary.New().Replace(t.Context(), filepath.Join(dir, "missing"), []byte("new")))
}
