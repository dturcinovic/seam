package git

import (
	"context"
	"errors"
	"strings"

	"github.com/dturcinovic/seam/internal/domain/change"
)

// WatchPaths implements out.WatchPathsReader.
func (r *Repo) WatchPaths(ctx context.Context) (change.WatchPaths, error) {
	dir, err := run(ctx, r.root, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return change.WatchPaths{}, err
	}
	ignored, err := r.ignoredDirs(ctx)
	if err != nil {
		return change.WatchPaths{}, err
	}
	return change.WatchPaths{Root: r.root, GitDir: strings.TrimSuffix(string(dir), "\n"), Ignored: ignored}, nil
}

// ignoredDirs lists the directories a pattern ignores. --directory names a
// directory of untracked files once, but also one that only holds ignored
// files without being ignored itself: a new file there would count, so
// check-ignore sorts those out.
func (r *Repo) ignoredDirs(ctx context.Context) ([]string, error) {
	out, err := run(ctx, r.root, "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory")
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, p := range splitNUL(out) {
		if strings.HasSuffix(p, "/") {
			dirs = append(dirs, p)
		}
	}
	return r.Ignored(ctx, dirs)
}

// Ignored implements out.IgnoreChecker. check-ignore takes a directory
// for one when it exists, so a pattern such as bin/ matches it without the
// slash.
func (r *Repo) Ignored(ctx context.Context, paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	args := []string{"check-ignore", "-z", "--stdin"}
	cmd := command(ctx, r.root, args...)
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	out, err := output(ctx, cmd, args)
	// Status 1 is check-ignore's answer that none is ignored.
	if gitErr, ok := errors.AsType[*commandError](err); ok && gitErr.code == 1 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return splitNUL(out), nil
}
