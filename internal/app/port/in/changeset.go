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
