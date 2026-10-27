package out

import (
	"context"

	"github.com/dturcinovic/seam/internal/domain/release"
)

//counterfeiter:generate . ReleaseFetcher

// ReleaseFetcher asks where seam is published for its latest release.
type ReleaseFetcher interface {
	LatestRelease(ctx context.Context) (release.Version, error)
}

// ReleaseStore keeps the last check for a newer release between sessions.
// A check never saved loads as the zero Check, one that cannot be read as
// the zero Check with the error.
type ReleaseStore interface {
	LoadCheck(ctx context.Context) (release.Check, error)
	SaveCheck(ctx context.Context, c release.Check) error
}
