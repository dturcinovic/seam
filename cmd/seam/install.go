package main

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
)

// repo is where seam is published: the release check asks it, and the
// notice sends whoever installed seam by hand there.
const repo = "dturcinovic/seam"

// updateCommand is how to update this install of seam. It looks at the
// running binary only once there is a newer release to tell of.
func updateCommand() string {
	pkg := ""
	if bi, ok := debug.ReadBuildInfo(); ok {
		pkg = bi.Path
	}
	return updateFor(executable(), goBins(), homebrewPrefixes(), pkg)
}

// updateFor tells from where binary lies how it was installed: under a
// Homebrew prefix, in a directory go install writes to, or anywhere else,
// which gets the release page.
func updateFor(binary string, goBins, homebrew []string, pkg string) string {
	if binary != "" && (strings.Contains(binary, "/Cellar/") || slices.ContainsFunc(homebrew, func(p string) bool {
		return p != "" && strings.HasPrefix(binary, filepath.Clean(p)+"/")
	})) {
		return "brew upgrade seam"
	}
	if binary != "" && pkg != "" && slices.ContainsFunc(goBins, func(d string) bool {
		return d != "" && filepath.Clean(d) == filepath.Dir(binary)
	}) {
		return "go install " + pkg + "@latest"
	}
	return "https://github.com/" + repo + "/releases/latest"
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

// homebrewPrefixes are where Homebrew installs: its own variable, else its
// defaults on macOS and Linux.
func homebrewPrefixes() []string {
	return []string{os.Getenv("HOMEBREW_PREFIX"), "/opt/homebrew", "/home/linuxbrew/.linuxbrew"}
}
