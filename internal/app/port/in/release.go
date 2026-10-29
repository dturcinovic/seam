package in

import (
	"context"

	"github.com/dturcinovic/seam/internal/domain/release"
)

//counterfeiter:generate . ReleaseNotifier

// ReleaseNotifier tells of a newer release of seam.
type ReleaseNotifier interface {
	// Check asks for the latest release in the background, if asking is on,
	// the running seam is a release and the last check is a day old. It
	// returns at once; seam stopping cancels it.
	Check()
	// Newer is the latest release the last check found, if it is newer
	// than the running one; it asks no one.
	Newer(ctx context.Context) (release.Version, bool)
}

//counterfeiter:generate . Upgrader

// Upgrader brings this install of seam to the latest release.
type Upgrader interface {
	// Upgrade upgrades as the install's kind allows; with check it only
	// tells what it would do. An error before the latest release is known
	// comes with the zero Upgrade.
	Upgrade(ctx context.Context, check bool) (release.Upgrade, error)
}
