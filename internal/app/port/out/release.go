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

//counterfeiter:generate . ReleaseAssets

// ReleaseAssets downloads the files of a release where seam is published.
type ReleaseAssets interface {
	// Asset reads file name of release v, at most max bytes; a larger one
	// fails.
	Asset(ctx context.Context, v release.Version, name string, max int64) ([]byte, error)
}

//counterfeiter:generate . BinaryReplacer

// BinaryReplacer replaces seam's own binary.
type BinaryReplacer interface {
	// Replace puts data in place of the file at path, with its mode, at
	// once or not at all: on any error the file is as it was.
	Replace(ctx context.Context, path string, data []byte) error
}

//counterfeiter:generate . CommandRunner

// CommandRunner runs the command that upgrades seam, with its output where
// the user sees it.
type CommandRunner interface {
	// Run runs name with args; a command that ran and failed is an error
	// whose chain has an ExitCode() int method.
	Run(ctx context.Context, name string, args ...string) error
}
