package git

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"go.uber.org/zap"

	"seam/internal/domain/change"
	"seam/internal/supervise"
)

// ownerMarker marks a session directory as seam's: one without it is never
// touched. It sits beside the checkouts rather than in one, as git worktree
// remove deletes a checkout whole, and an interrupted removal would leave a
// directory seam no longer recognises.
const ownerMarker = ".seam-owner"

// Checkouts makes checkouts of commits for one session and removes them
// with it. They lie in Dir/<repo>/<session>/<commit>, next to
// <session>.lock, which the session holds locked until it ends; a session
// whose lock can be taken has ended without removing them, and Clean does.
// It implements out.CommitCheckouts.
type Checkouts struct {
	root string
	// base holds the sessions of this repository.
	base  string
	log   *zap.Logger
	tasks *supervise.Group

	mu      sync.Mutex
	session *session
	made    map[change.ObjectID]string
	closed  bool
}

// session is the directory of this session's checkouts and its lock.
type session struct {
	dir  string
	lock *os.File
}

// ErrNoCheckouts is returned by Checkout when no directory for checkouts
// was configured.
var ErrNoCheckouts = errors.New("git: no directory for checkouts")

// NewCheckouts returns the checkouts of the repository at root, kept under
// dir. It touches nothing until Start or Checkout, and nothing at all
// without dir: a relative base would put checkouts wherever seam runs.
func NewCheckouts(root, dir string, log *zap.Logger, panics supervise.PanicFunc) *Checkouts {
	base := ""
	if dir != "" {
		sum := sha256.Sum256([]byte(root))
		base = filepath.Join(dir, hex.EncodeToString(sum[:8]))
	}
	return &Checkouts{
		root: root, base: base, log: log,
		tasks: supervise.NewGroup("git checkouts", panics, nil),
		made:  make(map[change.ObjectID]string),
	}
}

// Start cleans up after ended sessions in the background.
func (c *Checkouts) Start() error {
	return c.tasks.Go("clean", func(ctx context.Context) {
		if err := c.Clean(ctx); err != nil {
			c.log.Warn("clean up checkouts", zap.Error(err))
		}
	})
}

// Checkout implements out.CommitCheckouts. Its caller can only say that
// the checkout failed, so the reason goes to the log here.
func (c *Checkouts) Checkout(ctx context.Context, id change.ObjectID) (string, error) {
	dir, err := c.checkout(ctx, id)
	if err != nil {
		c.log.Warn("checkout failed", zap.Stringer("commit", id), zap.Error(err))
	}
	return dir, err
}

// checkout writes the tree without holding the lock, so checkouts of
// other commits run alongside and Close does not wait for one. What a
// failed one leaves in the session directory goes with it.
func (c *Checkouts) checkout(ctx context.Context, id change.ObjectID) (string, error) {
	dir, done, err := c.prepare(id)
	if err != nil || done {
		return dir, err
	}
	// A hover is no reason to run the repository's hooks or to download
	// large files: LFS objects stay pointers.
	args := []string{"-c", "core.hooksPath=/dev/null", "worktree", "add", "--detach", "--quiet", dir, id.String()}
	cmd := command(ctx, c.root, args...)
	cmd.Env = append(cmd.Env, "GIT_LFS_SKIP_SMUDGE=1")
	if _, err := output(ctx, cmd, args[2:]); err != nil {
		return "", fmt.Errorf("check out %s: %w", id, err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.made[id] = dir
	return dir, nil
}

// prepare opens the session if need be and names the directory of id's
// checkout; done says it is made already.
func (c *Checkouts) prepare(id change.ObjectID) (dir string, done bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case c.closed:
		return "", false, ErrClosed
	case c.base == "":
		return "", false, ErrNoCheckouts
	}
	if dir, ok := c.made[id]; ok {
		return dir, true, nil
	}
	if c.session == nil {
		if c.session, err = openSession(c.base, c.root); err != nil {
			return "", false, err
		}
	}
	return filepath.Join(c.session.dir, id.String()), false, nil
}

// Close removes this session's checkouts within the deadline of ctx. What
// it leaves, the next session's Clean removes once the lock is gone with
// this process.
func (c *Checkouts) Close(ctx context.Context) error {
	err := c.tasks.Stop(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.session == nil {
		return err
	}
	c.removeSession(ctx, c.session.dir)
	c.prune(ctx)
	return errors.Join(err, c.session.unlock())
}

// Clean removes the checkouts of every session of this repository that has
// ended: whose directory has the marker and whose lock can be taken.
func (c *Checkouts) Clean(ctx context.Context) error {
	if c.base == "" {
		return nil
	}
	entries, err := os.ReadDir(c.base)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list checkouts: %w", err)
	}
	cleaned := false
	for _, e := range entries {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		dir := filepath.Join(c.base, e.Name())
		if !e.IsDir() || !owned(dir) {
			continue
		}
		s, ok := takeLock(dir)
		if !ok {
			continue
		}
		c.log.Info("removing checkouts of an ended session", zap.String("dir", dir))
		c.removeSession(ctx, dir)
		cleaned = true
		if err := s.unlock(); err != nil {
			c.log.Warn("remove lock", zap.Error(err))
		}
	}
	if cleaned {
		c.prune(ctx)
	}
	return nil
}

// removeSession removes the checkouts in dir, then dir itself, its marker
// last: as long as anything is left, the marker says it is seam's.
func (c *Checkouts) removeSession(ctx context.Context, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		c.log.Warn("list session checkouts", zap.String("dir", dir), zap.Error(err))
		return
	}
	for _, e := range entries {
		if e.Name() != ownerMarker {
			c.removeCheckout(ctx, filepath.Join(dir, e.Name()))
		}
	}
	for _, path := range []string{filepath.Join(dir, ownerMarker), dir} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			c.log.Warn("remove session", zap.String("path", path), zap.Error(err))
			return
		}
	}
}

// removeCheckout removes a checkout with whatever was written into it.
// git forgets it first; a directory git no longer knows goes all the same,
// unless time is up: deleting a tree cannot be cut short, and the next
// session's Clean takes it.
func (c *Checkouts) removeCheckout(ctx context.Context, dir string) {
	_, err := run(ctx, c.root, "worktree", "remove", "--force", dir)
	if err == nil || ctx.Err() != nil {
		return
	}
	c.log.Debug("git worktree remove", zap.String("dir", dir), zap.Error(err))
	if err := os.RemoveAll(dir); err != nil {
		c.log.Warn("remove checkout", zap.String("dir", dir), zap.Error(err))
	}
}

// prune drops git's records of checkouts whose directory is gone.
func (c *Checkouts) prune(ctx context.Context) {
	if _, err := run(ctx, c.root, "worktree", "prune"); err != nil {
		c.log.Warn("git worktree prune", zap.Error(err))
	}
}

func owned(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ownerMarker))
	return err == nil
}

// openSession makes a session directory under base with its lock and
// marker. The lock comes first, so Clean never sees an unlocked session
// that is still being made.
func openSession(base, root string) (*session, error) {
	if err := os.MkdirAll(base, 0o700); err != nil {
		return nil, fmt.Errorf("make checkout directory: %w", err)
	}
	var id [8]byte
	_, _ = rand.Read(id[:]) // crypto/rand.Read never fails.
	dir := filepath.Join(base, hex.EncodeToString(id[:]))
	lock, err := os.OpenFile(dir+".lock", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("make session lock: %w", err)
	}
	s := &session{dir: dir, lock: lock}
	if err := tryLock(lock); err != nil {
		return nil, errors.Join(fmt.Errorf("lock session: %w", err), s.unlock())
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		return nil, errors.Join(fmt.Errorf("make session directory: %w", err), s.unlock())
	}
	if err := os.WriteFile(filepath.Join(dir, ownerMarker), []byte(root+"\n"), 0o600); err != nil {
		return nil, errors.Join(fmt.Errorf("mark session directory: %w", err), os.Remove(dir), s.unlock())
	}
	return s, nil
}

// takeLock locks the lock of the session at dir, if no running session
// holds it. A lock that is missing is made: the session is gone all the
// same.
func takeLock(dir string) (*session, bool) {
	lock, err := os.OpenFile(dir+".lock", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, false
	}
	if err := tryLock(lock); err != nil {
		_ = lock.Close() // Nothing was written.
		return nil, false
	}
	return &session{dir: dir, lock: lock}, true
}

// tryLock takes the lock without waiting for its holder.
func tryLock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// unlock removes the lock file and releases the lock, in this order: a
// session that finds the file once it is gone makes a new one, which no
// one else holds.
func (s *session) unlock() error {
	return errors.Join(os.Remove(s.lock.Name()), s.lock.Close())
}
