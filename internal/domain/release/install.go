package release

import (
	"cmp"
	"strings"
)

// Install is how seam was installed, which decides how it is upgraded. The
// zero value is not known: seam replaces nothing then, and the user
// upgrades by hand.
type Install uint8

// The kinds of install.
const (
	_ Install = iota
	// Homebrew installed seam; brew upgrades it.
	Homebrew
	// GoInstall built seam with go install; go install upgrades it.
	GoInstall
	// Package is a .deb or .rpm of a release. Its package manager owns the
	// binary, so seam never replaces it; without a package repository,
	// apt or dnf do not find a newer one either.
	Package
	// Archive is the binary of a release archive, put anywhere by hand.
	// seam replaces it itself.
	Archive
)

// Command is what upgrades an install of kind i to release v, if a command
// does: brew, or go install of module, seam's main package. go install gets
// v itself, as the module proxy may name an older @latest for a while
// after a release; zero v is @latest. Without module go install cannot be
// named.
func (i Install) Command(module string, v Version) ([]string, bool) {
	switch {
	case i == Homebrew:
		return []string{"brew", "upgrade", "seam"}, true
	case i == GoInstall && module != "":
		return []string{"go", "install", module + "@" + cmp.Or(v.String(), "latest")}, true
	}
	return nil, false
}

// Hint is how to update an install of kind i to release v, for a person
// to do: its command, seam upgrade for an archive, which it replaces
// itself, or else the page of the latest release.
func (i Install) Hint(module, page string, v Version) string {
	if cmd, ok := i.Command(module, v); ok {
		return strings.Join(cmd, " ")
	}
	if i == Archive {
		return "seam upgrade"
	}
	return page
}

// PackageCommands are the commands that install the packages of release v
// for goarch, named as nfpm's ConventionalFileName in .goreleaser.yaml
// names them: a .deb for apt, an .rpm for dnf.
func PackageCommands(v Version, goarch string) []string {
	n := strings.TrimPrefix(v.String(), "v")
	rpmArch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[goarch]
	return []string{
		"sudo apt install ./seam_" + n + "_" + goarch + ".deb",
		"sudo dnf install ./seam-" + n + "-1." + rpmArch + ".rpm",
	}
}
