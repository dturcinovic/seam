package statefile_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/adapter/driven/statefile"
	"github.com/dturcinovic/seam/internal/domain/release"
)

func TestAReleaseCheckIsKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seam", "release.json")
	store := statefile.New(statefile.Config{ReleaseFile: path})

	c, err := store.LoadCheck(t.Context())
	require.NoError(t, err)
	assert.Equal(t, release.Check{}, c, "never checked")

	latest, err := release.ParseVersion("v0.2.0")
	require.NoError(t, err)
	at := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	for _, want := range []release.Check{{Latest: latest, At: at}, {At: at.Add(time.Hour)}} {
		require.NoError(t, store.SaveCheck(t.Context(), want))
		got, err := store.LoadCheck(t.Context())
		require.NoError(t, err)
		assert.Equal(t, want.Latest, got.Latest)
		assert.True(t, want.At.Equal(got.At))
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temporary file is left")
}

func TestABrokenReleaseCheckIsAnError(t *testing.T) {
	tests := []struct{ name, content string }{
		{"no JSON", "{"},
		{"no release", `{"latest":"dev","checked_at":"2026-10-27T12:00:00Z"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "release.json")
			require.NoError(t, os.WriteFile(path, []byte(tt.content), 0o600))

			c, err := statefile.New(statefile.Config{ReleaseFile: path}).LoadCheck(t.Context())

			require.Error(t, err)
			assert.Zero(t, c, "the port's promise: the zero Check with the error")
		})
	}
}
