package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/release"
)

func TestWhatSeamUpgradeSays(t *testing.T) {
	v := func(s string) release.Version {
		r, err := release.ParseVersion(s)
		require.NoError(t, err)
		return r
	}
	up := func(o release.Outcome, cmds ...string) release.Upgrade {
		return release.Upgrade{Outcome: o, Running: v("v0.1.0"), Latest: v("v0.2.0"), Binary: "/opt/seam/seam", Commands: cmds}
	}
	const out = "seam v0.2.0 is out (you have v0.1.0): "
	tests := []struct {
		up   release.Upgrade
		want string
	}{
		{release.Upgrade{Outcome: release.Current, Running: v("v0.2.0"), Latest: v("v0.2.0")}, "seam v0.2.0 is the latest release"},
		{release.Upgrade{Outcome: release.Current, Running: v("v0.3.0"), Latest: v("v0.2.0")}, "seam v0.3.0 is newer than the latest release, v0.2.0"},
		{up(release.Ran, "brew upgrade seam"), "seam upgrade ran brew upgrade seam"},
		{up(release.Replaced), "seam upgraded from v0.1.0 to v0.2.0: /opt/seam/seam"},
		{up(release.WouldRun, "brew upgrade seam"), out + "seam upgrade runs brew upgrade seam"},
		{up(release.WouldReplace), out + "seam upgrade replaces /opt/seam/seam"},
		{up(release.ByPackage, "sudo apt install ./a.deb", "sudo dnf install ./a.rpm"),
			out + "its package manager owns seam; install the package of " + releasePage + ":\n  sudo apt install ./a.deb\n  sudo dnf install ./a.rpm"},
		{up(release.ByHand, "brew upgrade seam"), out + "run again: brew upgrade seam"},
		{up(release.ByHand), out + "upgrade by hand from " + releasePage},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, upgradeText(tt.up), "outcome %d", tt.up.Outcome)
	}
}

func TestADevBuildIsNotUpgraded(t *testing.T) {
	code, _, stderr := runSeam(t, "upgrade")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "no release build")
}
