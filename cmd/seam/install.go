package main

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/dturcinovic/seam/internal/domain/release"
)

// repo is where seam is published: the release check asks it, and the
// notice sends whoever installed seam by hand there.
const repo = "dturcinovic/seam"

// releasePage is the page of the latest release.
const releasePage = "https://github.com/" + repo + "/releases/latest"

// packageDir is where a .deb or .rpm of a release puts the binary.
const packageDir = "/usr/bin"

// thisInstall is how the running seam was installed, where its binary lies
// and the main package go install would build.
func thisInstall() (release.Install, string, string) {
	module := ""
	if bi, ok := debug.ReadBuildInfo(); ok {
		module = bi.Path
	}
	binary := executable()
	return installOf(binary, goBins(), archiveDirs()), binary, module
}

// updateCommand is how to update this install of seam to release latest.
// It looks at the running binary only once there is a newer release to
// tell of.
func updateCommand(latest release.Version) string {
	install, _, module := thisInstall()
	return install.Hint(module, releasePage, latest)
}

// installOf tells from where binary, its links resolved, lies how it was
// installed: in Homebrew's Cellar or Caskroom, in a directory go install
// writes to, where a package puts it, or in one of archiveDirs, unpacked by
// hand. Anywhere else it is not known: seam upgrade would replace a binary
// that a version manager such as mise, asdf or aqua, or a build in a
// clone, keeps track of. Homebrew's own bin holds only links into its
// Cellar and Caskroom, so its prefix tells nothing, and matching it would
// take a binary unpacked into /usr/local/bin on an Intel Mac for
// Homebrew's.
func installOf(binary string, goBins, archiveDirs []string) release.Install {
	switch {
	case binary == "":
		return 0
	case strings.Contains(binary, "/Cellar/") || strings.Contains(binary, "/Caskroom/"):
		return release.Homebrew
	case slices.ContainsFunc(goBins, func(d string) bool { return d != "" && filepath.Clean(d) == filepath.Dir(binary) }):
		return release.GoInstall
	case filepath.Dir(binary) == packageDir:
		return release.Package
	case slices.Contains(archiveDirs, filepath.Dir(binary)):
		return release.Archive
	}
	return 0
}

// archiveDirs are where people put the binary of a release archive, as the
// README shows: the user's own bin directories and /usr/local/bin, with
// their links resolved as the binary's path is.
func archiveDirs() []string {
	dirs := []string{resolved("/usr/local/bin")}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, resolved(filepath.Join(home, ".local", "bin")), resolved(filepath.Join(home, "bin")))
	}
	return dirs
}

// executable is the running binary. A Homebrew install runs through a link
// into its Cellar, which only the resolved path shows.
func executable() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved
	}
	return exe
}

// goBins are where go install puts a binary: GOBIN, else the bin of every
// GOPATH entry, which defaults to ~/go.
func goBins() []string {
	if gobin := os.Getenv("GOBIN"); gobin != "" {
		return []string{resolved(gobin)}
	}
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		gopath = filepath.Join(home, "go")
	}
	var bins []string
	for _, p := range filepath.SplitList(gopath) {
		bins = append(bins, resolved(filepath.Join(p, "bin")))
	}
	return bins
}

// resolved is dir with its links resolved, as the binary's path is, or dir
// where that fails.
func resolved(dir string) string {
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		return r
	}
	return dir
}
