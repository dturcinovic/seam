package main

import (
	"cmp"
	"runtime/debug"
	"strings"
)

// version is the release tag, stamped by `task build` and the release
// build. Commit and date need no stamp: go build records them for a build
// inside a git checkout.
var version = "" //nolint:gochecknoglobals // set through -ldflags -X, which needs a variable

func versionString() string {
	bi, _ := debug.ReadBuildInfo()
	return describe(version, bi)
}

// describe is the version as --version prints it, such as
// "v0.1.0 (1eac307, 2026-10-25)". A local build without a stamp is "dev",
// so it never claims a release; a module that go install downloaded has no
// commit, only its version.
func describe(stamped string, bi *debug.BuildInfo) string {
	if bi == nil {
		return cmp.Or(stamped, "dev")
	}
	var commit, date string
	modified := false
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			commit = s.Value[:min(len(s.Value), 7)]
		case "vcs.time":
			date, _, _ = strings.Cut(s.Value, "T")
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	v := stamped
	if v == "" && commit == "" && bi.Main.Version != "(devel)" {
		v = bi.Main.Version
	}
	v = cmp.Or(v, "dev")
	if commit == "" {
		return v
	}
	if modified {
		commit += "-dirty"
	}
	if date == "" {
		return v + " (" + commit + ")"
	}
	return v + " (" + commit + ", " + date + ")"
}
