package release

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrNoRelease is returned by ParseVersion for anything but a release tag.
var ErrNoRelease = errors.New("no release version")

// Version is a release tag vMAJOR.MINOR.PATCH. The zero value is no release.
type Version struct {
	major, minor, patch int
}

// ParseVersion reads a release tag. A build between releases, as git
// describe names it (v0.1.0-12-g3164b9b), or a dev build is no release: it
// must never be told that the release it was built after is newer. Nor is
// v0.0.0, which is the zero value.
func ParseVersion(s string) (Version, error) {
	parts := strings.Split(strings.TrimPrefix(s, "v"), ".")
	if !strings.HasPrefix(s, "v") || len(parts) != 3 {
		return Version{}, fmt.Errorf("%w: %q", ErrNoRelease, s)
	}
	var n [3]int
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 || strconv.Itoa(v) != p {
			return Version{}, fmt.Errorf("%w: %q", ErrNoRelease, s)
		}
		n[i] = v
	}
	v := Version{n[0], n[1], n[2]}
	if v.IsZero() {
		return Version{}, fmt.Errorf("%w: %q", ErrNoRelease, s)
	}
	return v, nil
}

// IsZero reports whether v is no release.
func (v Version) IsZero() bool { return v == Version{} }

// String is the tag, empty for no release.
func (v Version) String() string {
	if v.IsZero() {
		return ""
	}
	return fmt.Sprintf("v%d.%d.%d", v.major, v.minor, v.patch)
}

// Newer reports whether v is a later release than o.
func (v Version) Newer(o Version) bool {
	if v.major != o.major {
		return v.major > o.major
	}
	if v.minor != o.minor {
		return v.minor > o.minor
	}
	return v.patch > o.patch
}

// Interval is how often seam asks for the latest release at most.
const Interval = 24 * time.Hour

// Check is the outcome of the last time seam asked for the latest release.
// Latest stays as it was when asking failed; At is set either way, so a
// failing network is asked no more often than a working one.
type Check struct {
	Latest Version
	At     time.Time
}

// Due reports whether it is time to ask again. A check from the future, as
// after the clock was set back, counts as due rather than blocking for long.
func (c Check) Due(now time.Time) bool {
	since := now.Sub(c.At)
	return c.At.IsZero() || since >= Interval || since < 0
}

// Notice is the line that tells of latest, a newer release than running,
// and how to update to it.
func Notice(running, latest Version, update string) string {
	return fmt.Sprintf("seam %s is out (you have %s): %s", latest, running, update)
}
