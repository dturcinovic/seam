package release

import "errors"

// ErrNotARelease is seam upgrade in a build that is no release, such as
// dev: it would replace a build of its own with one it did not choose.
var ErrNotARelease = errors.New("this seam is no release build")

// Outcome is what seam upgrade did, or why it left the upgrade to the user.
type Outcome uint8

// The outcomes. The zero value is none: seam upgrade failed before it knew
// the latest release.
const (
	_ Outcome = iota
	// Current is a seam that is the latest release already.
	Current
	// Ran is the install's command run to its end; what it installed is
	// its own to say.
	Ran
	// Replaced is an archive's binary replaced by the latest release.
	Replaced
	// WouldRun is --check of an install with a command: Commands.
	WouldRun
	// WouldReplace is --check of an archive.
	WouldReplace
	// ByPackage is a .deb or .rpm, whose package manager owns the binary:
	// Commands install the packages of the latest release.
	ByPackage
	// ByHand is an upgrade seam could not do: the command that failed in
	// Commands to run again, or none for an archive or an install not
	// known, from the release page.
	ByHand
)

// Upgrade is the outcome of seam upgrade on Binary, a Running release, to
// the Latest one.
type Upgrade struct {
	Outcome         Outcome
	Running, Latest Version
	Binary          string
	Commands        []string
}
