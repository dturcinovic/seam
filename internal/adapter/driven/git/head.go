package git

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dturcinovic/seam/internal/domain/change"
)

// Head reads what HEAD of a worktree stands on. It implements
// out.HeadReader apart from Repo, which `seam mcp` does not open: each
// call is one or two short git commands.
type Head struct {
	root string
}

// NewHead reads HEAD of the worktree at root.
func NewHead(root string) *Head { return &Head{root: root} }

const branchPrefix = "refs/heads/"

// ReadHead implements out.HeadReader.
func (h *Head) ReadHead(ctx context.Context) (change.Head, error) {
	out, err := run(ctx, h.root, "symbolic-ref", "-q", "HEAD")
	if err == nil {
		return change.Head{Branch: strings.TrimPrefix(strings.TrimSpace(string(out)), branchPrefix)}, nil
	}
	// symbolic-ref exits 1 for a detached HEAD, which a rebase makes too.
	if gitErr, ok := errors.AsType[*commandError](err); !ok || gitErr.code != 1 {
		return change.Head{}, err
	}
	if branch, err := h.rebasing(ctx); err != nil || branch != "" {
		return change.Head{Branch: branch}, err
	}
	id, err := resolveCommit(ctx, h.root, "HEAD")
	if err != nil {
		return change.Head{}, err
	}
	return change.Head{Commit: id}, nil
}

// rebasing returns the branch a rebase in progress works on, as git keeps
// it in head-name: rebase-merge for the merge backend, rebase-apply for
// the apply backend; empty for none. A rebase of a detached HEAD writes
// "detached HEAD" there, which names no branch.
func (h *Head) rebasing(ctx context.Context) (string, error) {
	out, err := run(ctx, h.root, "rev-parse", "--git-path", "rebase-merge/head-name", "--git-path", "rebase-apply/head-name")
	if err != nil {
		return "", err
	}
	for line := range strings.Lines(string(out)) {
		name := strings.TrimSuffix(line, "\n")
		if !filepath.IsAbs(name) {
			name = filepath.Join(h.root, name)
		}
		data, err := os.ReadFile(name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("read rebase: %w", err)
		}
		if ref, ok := strings.CutPrefix(strings.TrimSpace(string(data)), branchPrefix); ok {
			return ref, nil
		}
	}
	return "", nil
}

// ListBranches implements out.HeadReader.
func (h *Head) ListBranches(ctx context.Context) ([]string, error) {
	out, err := run(ctx, h.root, "for-each-ref", "--format=%(refname:lstrip=2)", branchPrefix)
	if err != nil {
		return nil, err
	}
	var names []string
	for line := range strings.Lines(string(out)) {
		names = append(names, strings.TrimSuffix(line, "\n"))
	}
	return names, nil
}
