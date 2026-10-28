package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/testsupport"
)

func TestUpdateFor(t *testing.T) {
	const pkg = "github.com/dturcinovic/seam/cmd/seam"
	goBins := []string{"/home/u/go/bin/", "", "/opt/gobin"}
	homebrew := []string{"", "/opt/homebrew", "/home/linuxbrew/.linuxbrew"}
	tests := []struct {
		binary string
		want   string
	}{
		{"/opt/homebrew/bin/seam", "brew upgrade seam"},
		{"/usr/local/Cellar/seam/0.1.0/bin/seam", "brew upgrade seam"},
		{"/home/linuxbrew/.linuxbrew/bin/seam", "brew upgrade seam"},
		{"/opt/homebrewer/seam", "https://github.com/dturcinovic/seam/releases/latest"},
		{"/home/u/go/bin/seam", "go install " + pkg + "@latest"},
		{"/opt/gobin/seam", "go install " + pkg + "@latest"},
		{"/usr/bin/seam", "https://github.com/dturcinovic/seam/releases/latest"},
		{"", "https://github.com/dturcinovic/seam/releases/latest"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, updateFor(tt.binary, goBins, homebrew, pkg), tt.binary)
	}
	assert.Equal(t, "https://github.com/dturcinovic/seam/releases/latest", updateFor("/opt/gobin/seam", goBins, homebrew, ""),
		"without its package path go install cannot be named")
}

func TestWhereGoInstallWrites(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tests := []struct {
		name, gobin, gopath string
		want                []string
	}{
		{"GOBIN", "/opt/gobin", "/ignored", []string{"/opt/gobin"}},
		{"GOPATH entries", "", "/a" + string(filepath.ListSeparator) + "/b", []string{"/a/bin", "/b/bin"}},
		{"the default", "", "", []string{filepath.Join(home, "go", "bin")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GOBIN", tt.gobin)
			t.Setenv("GOPATH", tt.gopath)
			assert.Equal(t, tt.want, goBins())
		})
	}
}

func TestALinkedGOPATHIsResolved(t *testing.T) {
	real := testsupport.RealTempDir(t)
	require.NoError(t, os.Mkdir(filepath.Join(real, "bin"), 0o700))
	link := filepath.Join(t.TempDir(), "go")
	require.NoError(t, os.Symlink(real, link))
	t.Setenv("GOBIN", "")
	t.Setenv("GOPATH", link)

	assert.Equal(t, []string{filepath.Join(real, "bin")}, goBins(), "as the binary's own path is")
}
