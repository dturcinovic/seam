package fswatch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"

	"seam/internal/app/port/out"
	"seam/internal/domain/change"
)

// Watcher implements out.ChangeWatcher.
type Watcher struct {
	log *zap.Logger
}

// New returns a watcher that logs why watching stopped.
func New(log *zap.Logger) *Watcher {
	return &Watcher{log: log}
}

// Watch implements out.ChangeWatcher.
func (w *Watcher) Watch(ctx context.Context, p change.WatchPaths, worktree bool, l out.WatchListener) error {
	err := watch(ctx, p, worktree, l)
	if err != nil {
		w.log.Warn("watcher stopped", zap.Error(err))
	}
	return err
}

func watch(ctx context.Context, p change.WatchPaths, worktree bool, l out.WatchListener) error {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("start watching: %w", limitHint(err))
	}
	// Closing ends the watcher's own goroutine; what it reports then is of
	// no use to anyone.
	defer func() { _ = fw.Close() }()
	// The list grows with directories ignored after the start; the
	// caller's stays as it was.
	p.Ignored = slices.Clone(p.Ignored)
	t := &tree{fw: fw, paths: p, worktree: worktree, l: l}
	if err := t.setUp(); err != nil {
		return err
	}
	// What changed while the watches were being set up came after the
	// caller's last look and before any event could tell.
	l.Changed()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-fw.Events:
			if err := t.handle(ev); err != nil {
				return err
			}
		case err := <-fw.Errors:
			// A full queue lost events, not the watches: a reload sees
			// whatever they were.
			if !errors.Is(err, fsnotify.ErrEventOverflow) {
				return fmt.Errorf("watch: %w", err)
			}
			l.Changed()
		}
	}
}

// tree is the watched directories of a repository and who hears of them.
type tree struct {
	fw       *fsnotify.Watcher
	paths    change.WatchPaths
	worktree bool
	l        out.WatchListener
}

// setUp watches git's directory and its reflog, whose HEAD grows with
// every commit, reset and checkout, even one that only moves a branch; git
// writes index and HEAD to a lock file and renames it into place, which
// would end a watch on the file itself, so the directories see them. With
// the worktree it watches its directories too.
func (t *tree) setUp() error {
	if err := t.add(t.paths.GitDir); err != nil {
		return err
	}
	if err := t.add(filepath.Join(t.paths.GitDir, "logs")); err != nil {
		return err
	}
	if !t.worktree {
		return nil
	}
	dirs, err := t.dirs(t.paths.Root)
	if err != nil {
		return err
	}
	return t.addAll(dirs)
}

// handle reports ev if it changes what git sees, and watches a directory
// that came into the worktree unless git ignores it.
func (t *tree) handle(ev fsnotify.Event) error {
	switch dir, name := filepath.Split(ev.Name); {
	case dir == t.paths.GitDir+"/" && (name == "index" || name == "HEAD"),
		dir == filepath.Join(t.paths.GitDir, "logs")+"/" && name == "HEAD":
		t.l.Changed()
		return nil
	case !t.worktree || t.skipped(ev.Name):
		return nil
	}
	if ev.Has(fsnotify.Create) {
		ignored, err := t.addNew(ev.Name)
		if err != nil {
			return err
		}
		if ignored {
			return nil
		}
	}
	t.l.Changed()
	return nil
}

// addNew watches the directories of a tree that came into the worktree.
// It asks git about the tree first, so an ignored one, such as a fresh
// node_modules, is never walked, then about the directories below it all
// at once; ignored says git ignores the tree. A file, or a tree gone
// again, does nothing: what it held before its watch came is in the reload
// its creation leads to.
func (t *tree) addNew(root string) (ignored bool, err error) {
	if info, err := os.Lstat(root); err != nil || !info.IsDir() {
		return false, nil //nolint:nilerr // gone again, see above
	}
	if t.ignore(t.l.Ignored([]string{root})) {
		return true, nil
	}
	dirs, err := t.dirs(root)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	t.ignore(t.l.Ignored(dirs[1:]))
	return false, t.addAll(dirs)
}

// ignore leaves dirs out from now on and says whether there were any.
func (t *tree) ignore(dirs []string) bool {
	for _, dir := range dirs {
		if rel, err := filepath.Rel(t.paths.Root, dir); err == nil {
			t.paths.Ignored = append(t.paths.Ignored, filepath.ToSlash(rel)+"/")
		}
	}
	return len(dirs) > 0
}

// dirs lists dir and the directories below it, but git's own and the
// ignored ones; a file has none.
func (t *tree) dirs(dir string) ([]string, error) {
	var dirs []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil && path == dir:
			return err
		case err != nil:
			// A directory that cannot be read cannot be watched either.
			return nil
		case !d.IsDir():
			return nil
		case path != dir && t.skipped(path):
			return filepath.SkipDir
		}
		dirs = append(dirs, path)
		return nil
	})
	return dirs, err
}

// addAll watches dirs but those ignored meanwhile.
func (t *tree) addAll(dirs []string) error {
	for _, dir := range dirs {
		if dir != t.paths.Root && t.skipped(dir) {
			continue
		}
		if err := t.add(dir); err != nil {
			return err
		}
	}
	return nil
}

// add watches dir. Only git's directory and the root must exist: any other
// may be gone before its watch comes, and its removal is an event of the
// one above.
func (t *tree) add(dir string) error {
	err := t.fw.Add(dir)
	if err != nil && (dir == t.paths.GitDir || dir == t.paths.Root || !errors.Is(err, fs.ErrNotExist)) {
		return fmt.Errorf("watch %s: %w", dir, limitHint(err))
	}
	return nil
}

// skipped says whether path lies in git's directory or an ignored one.
func (t *tree) skipped(path string) bool {
	rel, err := filepath.Rel(t.paths.Root, path)
	if err != nil {
		return true
	}
	dir := filepath.ToSlash(rel) + "/"
	if strings.HasPrefix(dir, ".git/") || strings.HasPrefix(path+"/", t.paths.GitDir+"/") {
		return true
	}
	return slices.ContainsFunc(t.paths.Ignored, func(ignored string) bool { return strings.HasPrefix(dir, ignored) })
}

// limitHint names the setting to raise when the kernel runs out of watches
// or watchers.
func limitHint(err error) error {
	switch {
	case errors.Is(err, syscall.ENOSPC):
		return fmt.Errorf("%w (raise fs.inotify.max_user_watches)", err)
	case errors.Is(err, syscall.EMFILE):
		return fmt.Errorf("%w (raise fs.inotify.max_user_instances)", err)
	}
	return err
}
