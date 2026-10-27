package usecase

import (
	"context"
	"time"

	"github.com/dturcinovic/seam/internal/app/port/out"
	"github.com/dturcinovic/seam/internal/domain/release"
	"github.com/dturcinovic/seam/internal/supervise"
)

// ReleaseSettings are what the release check knows of the running seam.
type ReleaseSettings struct {
	// Running is this seam's release, zero for a build that is no release:
	// it has no version to compare.
	Running release.Version
	// Ask is false where the user turned the check off.
	Ask bool
}

func (s ReleaseSettings) asks() bool {
	return s.Ask && !s.Running.IsZero()
}

// ReleaseChecks implements in.ReleaseNotifier. A check that fails says
// nothing: seam works the same without it, and the next one is a day later.
type ReleaseChecks struct {
	settings ReleaseSettings
	fetcher  out.ReleaseFetcher
	store    out.ReleaseStore
	group    *supervise.Group
}

// NewReleaseChecks returns the use case for the running seam.
func NewReleaseChecks(s ReleaseSettings, f out.ReleaseFetcher, st out.ReleaseStore, panics supervise.PanicFunc) *ReleaseChecks {
	return &ReleaseChecks{settings: s, fetcher: f, store: st, group: supervise.NewGroup("release", panics, nil)}
}

// Check implements in.ReleaseNotifier.
func (r *ReleaseChecks) Check() {
	if !r.settings.asks() {
		return
	}
	// Go fails only once Stop has run, when no one waits for a check.
	_ = r.group.Go("check", r.check)
}

// Newer implements in.ReleaseNotifier.
func (r *ReleaseChecks) Newer(ctx context.Context) (release.Version, bool) {
	return NewerRelease(ctx, r.settings, r.store)
}

// Stop cancels a check still running.
func (r *ReleaseChecks) Stop(ctx context.Context) error {
	return r.group.Stop(ctx)
}

// NewerRelease is Newer for a caller that only reads the last check, such
// as --version, without a session.
func NewerRelease(ctx context.Context, s ReleaseSettings, store out.ReleaseStore) (release.Version, bool) {
	if !s.asks() {
		return release.Version{}, false
	}
	c, err := store.LoadCheck(ctx)
	if err != nil || !c.Latest.Newer(s.Running) {
		return release.Version{}, false
	}
	return c.Latest, true
}

// check records the attempt before asking, so a question that never ends,
// as on a network that drops it, still counts as the day's one. A record
// that cannot be read is the zero Check, which asking replaces.
func (r *ReleaseChecks) check(ctx context.Context) {
	now := time.Now()
	last, err := r.store.LoadCheck(ctx)
	if err == nil && !last.Due(now) {
		return
	}
	if r.store.SaveCheck(ctx, release.Check{Latest: last.Latest, At: now}) != nil {
		// Without the record every start would ask again.
		return
	}
	latest, err := r.fetcher.LatestRelease(ctx)
	if err != nil {
		return
	}
	// Failing here keeps the attempt; the answer is asked for again tomorrow.
	_ = r.store.SaveCheck(ctx, release.Check{Latest: latest, At: now})
}
