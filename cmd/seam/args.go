package main

import (
	"fmt"

	"seam/internal/domain/change"
)

// comparison maps the command line to the sides to compare.
func comparison(args []string, staged bool) (change.Comparison, error) {
	switch {
	case staged && len(args) > 0:
		return change.Comparison{}, fmt.Errorf("%w: --staged compares HEAD with the index and takes no revision", change.ErrInvalidComparison)
	case staged:
		return change.NewComparison(change.Commit("HEAD"), change.Index())
	case len(args) == 0:
		return change.NewComparison(change.Commit("HEAD"), change.Worktree())
	case len(args) == 1:
		return change.NewComparison(change.Commit(args[0]), change.Worktree())
	}
	return change.NewComparison(change.Commit(args[0]), change.Commit(args[1]))
}
