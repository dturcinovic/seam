package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// outsideRepo runs the test from a directory that is in no repository.
func outsideRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Chdir(dir)
	return dir
}

func put(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestFilesOnDiskOutsideARepository(t *testing.T) {
	dir := outsideRepo(t)
	put(t, filepath.Join(dir, "l/same.go"), "a\n")
	put(t, filepath.Join(dir, "l/old.go"), "a\n")
	put(t, filepath.Join(dir, "l/changed.go"), "a\n")
	put(t, filepath.Join(dir, "r/same.go"), "a\n")
	put(t, filepath.Join(dir, "r/changed.go"), "b\n")
	put(t, filepath.Join(dir, "r/new.go"), "b\n")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"two directories", []string{"l", "r"}, "M\tchanged.go\nA\tnew.go\nD\told.go\n"},
		{"two files", []string{"l/changed.go", "r/changed.go"}, "M\tr/changed.go\n"},
		{"an added file", []string{os.DevNull, "r/new.go"}, "A\tr/new.go\n"},
		{"a deleted file", []string{"l/old.go", os.DevNull}, "D\tl/old.go\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runSeam(t, append([]string{"--files"}, tt.args...)...)

			require.Equal(t, 0, code, stderr)
			assert.Equal(t, tt.want, stdout)
		})
	}
}

func TestFilesOnDiskThatCannotBeCompared(t *testing.T) {
	dir := outsideRepo(t)
	put(t, filepath.Join(dir, "a.go"), "a\n")
	put(t, filepath.Join(dir, "d/b.go"), "b\n")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"one path", []string{"--files", "a.go"}, "two files or two directories"},
		{"a file and a directory", []string{"--files", "a.go", "d"}, "are a file and a directory"},
		{"a missing path", []string{"--files", "a.go", "missing.go"}, "missing.go"},
		{"both missing", []string{"--files", os.DevNull, os.DevNull}, "both sides are missing"},
		{"with --staged", []string{"--files", "--staged", "a.go", "d/b.go"}, "takes no --staged"},
		{"with --watch", []string{"--files", "--watch", "a.go", "d/b.go"}, "files on disk"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, stderr := runSeam(t, tt.args...)

			assert.Equal(t, 1, code)
			assert.Contains(t, stderr, tt.want)
		})
	}
}
