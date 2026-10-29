package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/release"
	"github.com/dturcinovic/seam/internal/testsupport"
)

func TestInstallOf(t *testing.T) {
	goBins := []string{"/home/u/go/bin/", "", "/opt/gobin"}
	archiveDirs := []string{"/usr/local/bin", "/home/u/.local/bin", "/home/u/bin"}
	tests := []struct {
		binary string
		want   release.Install
	}{
		{"/opt/homebrew/Caskroom/seam/0.1.0/seam", release.Homebrew},
		{"/usr/local/Cellar/seam/0.1.0/bin/seam", release.Homebrew},
		{"/usr/local/Caskroom/seam/0.1.0/seam", release.Homebrew},
		{"/home/linuxbrew/.linuxbrew/Cellar/seam/0.1.0/bin/seam", release.Homebrew},
		{"/usr/local/bin/seam", release.Archive},
		{"/home/u/bin/seam", release.Archive},
		{"/opt/homebrewer/seam", 0},
		{"/home/u/.local/share/mise/installs/seam/0.1.0/seam", 0},
		{"/home/u/src/seam/bin/seam", 0},
		{"/home/u/go/bin/seam", release.GoInstall},
		{"/opt/gobin/seam", release.GoInstall},
		{"/usr/bin/seam", release.Package},
		{"/home/u/.local/bin/seam", release.Archive},
		{"", 0},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, installOf(tt.binary, goBins, archiveDirs), tt.binary)
	}
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
