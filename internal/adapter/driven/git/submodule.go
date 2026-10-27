package git

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dturcinovic/seam/internal/domain/change"
)

// ReadSubmodule implements out.SubmoduleReader. Without a .git of its own the
// submodule is not checked out, and git run in its directory would answer
// for this repository instead.
func (r *Repo) ReadSubmodule(ctx context.Context, path string) (change.ObjectID, bool, error) {
	if _, err := r.files.Lstat(filepath.Join(path, ".git")); err != nil {
		return change.ObjectID{}, false, worktreeError(path, err)
	}
	commit, dirty, err := r.submodule(ctx, path)
	if err != nil {
		return change.ObjectID{}, false, fmt.Errorf("submodule %s: %w", path, err)
	}
	return commit, dirty, nil
}

// submodule asks the submodule for its commit, and this repository whether
// it is dirty: only this repository knows the submodule's ignore setting,
// which git diff obeys.
func (r *Repo) submodule(ctx context.Context, path string) (change.ObjectID, bool, error) {
	head, err := run(ctx, filepath.Join(r.root, path), "rev-parse", "HEAD")
	if err != nil {
		return change.ObjectID{}, false, err
	}
	commit, err := change.ParseObjectID(strings.TrimSuffix(string(head), "\n"))
	if err != nil {
		return change.ObjectID{}, false, err
	}
	status, err := run(ctx, r.root, "status", "--porcelain=v2", "--untracked-files=no", "--", ":(literal)"+path)
	if err != nil {
		return change.ObjectID{}, false, err
	}
	return commit, modifiedContent(string(status)), nil
}

// modifiedContent reads the submodule field "S<c><m><u>" of a changed entry
// of `git status --porcelain=v2`: m is M when tracked files of the submodule
// have changes. Untracked ones (u) do not count, as for git diff.
func modifiedContent(status string) bool {
	for line := range strings.Lines(status) {
		fields := strings.Fields(line)
		if len(fields) > 2 && (fields[0] == "1" || fields[0] == "2") && strings.HasPrefix(fields[2], "S") {
			return len(fields[2]) == 4 && fields[2][2] == 'M'
		}
	}
	return false
}
