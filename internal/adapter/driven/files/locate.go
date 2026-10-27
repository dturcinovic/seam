package files

import (
	"path/filepath"
	"strings"
)

// LocateInWorktree implements out.WorktreeLocator. The path is resolved, as
// git difftool hands over links into the worktree.
func (d *Disk) LocateInWorktree(path string) (string, bool) {
	if d.root == "" {
		return "", false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(d.root, real)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") || rel == ".git" || strings.HasPrefix(rel, ".git/") {
		return "", false
	}
	return rel, true
}
