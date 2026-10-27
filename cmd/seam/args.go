package main

import (
	"fmt"
	"os"

	"github.com/dturcinovic/seam/internal/domain/change"
)

// comparison maps the command line to the sides to compare.
func comparison(args []string, staged, onDisk bool) (change.Comparison, error) {
	switch {
	case onDisk && staged:
		return change.Comparison{}, fmt.Errorf("%w: --files compares files on disk and takes no --staged", change.ErrInvalidComparison)
	case onDisk:
		return diskComparison(args)
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

// diskComparison compares two files or two directories, as git difftool
// hands them over.
func diskComparison(args []string) (change.Comparison, error) {
	if len(args) != 2 {
		return change.Comparison{}, fmt.Errorf("%w: --files takes two files or two directories", change.ErrInvalidComparison)
	}
	left, err := diskSide(args[0])
	if err != nil {
		return change.Comparison{}, err
	}
	right, err := diskSide(args[1])
	if err != nil {
		return change.Comparison{}, err
	}
	return change.NewComparison(left, right)
}

// diskSide is a file or directory on disk. /dev/null is a side that does not
// exist: git difftool passes it for an added or deleted file.
func diskSide(path string) (change.Source, error) {
	if path == os.DevNull {
		return change.File(""), nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return change.Source{}, fmt.Errorf("--files: %w", err)
	}
	if info.IsDir() {
		return change.Dir(path), nil
	}
	return change.File(path), nil
}
