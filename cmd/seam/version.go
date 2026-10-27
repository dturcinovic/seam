package main

import (
	"cmp"
	"runtime/debug"
	"strings"

	"github.com/dturcinovic/seam/internal/domain/release"
)

// version is the release tag, stamped by `task build` and the release
// build. Commit and date need no stamp: go build records them for a build
// inside a git checkout.
var version = "" //nolint:gochecknoglobals // set through -ldflags -X, which needs a variable

// build is what this binary knows of itself, read once for both the version
// it prints and the release it compares, so the two cannot disagree.
type build struct {
	stamped, module, commit, date string
	modified                      bool
}

func thisBuild() build {
	bi, _ := debug.ReadBuildInfo()
	return readBuild(version, bi)
}

func readBuild(stamped string, bi *debug.BuildInfo) build {
	b := build{stamped: stamped}
	if bi == nil {
		return b
	}
	b.module = bi.Main.Version
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			b.commit = s.Value[:min(len(s.Value), 7)]
		case "vcs.time":
			b.date, _, _ = strings.Cut(s.Value, "T")
		case "vcs.modified":
			b.modified = s.Value == "true"
		}
	}
	return b
}

// tag is the version without commit and date. A local build without a
// stamp is "dev", so it never claims a release; a module that go install
// downloaded has no commit, only its version.
func (b build) tag() string {
	v := b.stamped
	if v == "" && b.commit == "" && b.module != "(devel)" {
		v = b.module
	}
	return cmp.Or(v, "dev")
}

// String is the version as --version prints it, such as
// "v0.1.0 (1eac307, 2026-10-25)".
func (b build) String() string {
	if b.commit == "" {
		return b.tag()
	}
	commit := b.commit
	if b.modified {
		commit += "-dirty"
	}
	if b.date == "" {
		return b.tag() + " (" + commit + ")"
	}
	return b.tag() + " (" + commit + ", " + b.date + ")"
}

// release is the release this build is, zero for any other: a build with
// changes of its own is no release even on the tag.
func (b build) release() release.Version {
	if b.modified {
		return release.Version{}
	}
	r, err := release.ParseVersion(b.tag())
	if err != nil {
		return release.Version{}
	}
	return r
}
