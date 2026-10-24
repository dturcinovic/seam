package change

import "errors"

// Errors of reloading.
var (
	// ErrNotLoaded means a reload came before the first load: there are no
	// resolved commits to compare against yet.
	ErrNotLoaded = errors.New("changeset not loaded yet")
	// ErrReloaderFailed is the error of every reload once the reloader has
	// failed: its background work panicked and runs no more.
	ErrReloaderFailed = errors.New("changeset reloader failed")
)

// Gen is a generation of reloading the changeset. The UI hands out strictly
// increasing ones and keeps only the update of the newest.
type Gen uint64

// Update is the outcome of a reload: the changeset, or Err if it failed.
type Update struct {
	Gen       Gen
	Changeset Changeset
	Err       error
}
