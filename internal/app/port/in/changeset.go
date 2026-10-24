package in

import (
	"context"

	"seam/internal/domain/change"
)

// ChangesetLoader loads the files that differ in a comparison. It is called
// once, before the TUI starts; an error ends seam.
type ChangesetLoader interface {
	// LoadChangeset resolves the comparison's commits and returns them with
	// the changed files, untracked ones included when the right side is the
	// worktree.
	LoadChangeset(ctx context.Context, c change.Comparison) (change.Changeset, error)
}

// ChangesetReloader reloads the changeset while the TUI runs, against the
// commits the first load resolved. Reload never blocks: the work runs in the
// background, and its outcome waits in Drain until the UI collects it.
type ChangesetReloader interface {
	// Reload starts a reload and replaces one still waiting.
	Reload(gen change.Gen)
	// Signal is set, without blocking, after each new update.
	Signal() <-chan struct{}
	// Drain hands over the updates since the last call, oldest first.
	Drain() []change.Update
}
