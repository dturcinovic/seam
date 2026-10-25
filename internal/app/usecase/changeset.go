package usecase

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/dturcinovic/seam/internal/app/port/out"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/supervise"
)

// Changesets implements in.ChangesetLoader and in.ChangesetReloader.
type Changesets struct {
	*worker[change.Gen, change.Update]

	revisions out.RevisionResolver
	changes   out.ChangeLister
	untracked out.UntrackedLister

	mu sync.Mutex
	// asked is the comparison as the user named it, which each reload
	// resolves anew.
	asked *change.Comparison
}

// NewChangesets returns the use case over its ports. Its reload worker runs
// between Start and Stop.
func NewChangesets(r out.RevisionResolver, c out.ChangeLister, u out.UntrackedLister, panics supervise.PanicFunc) *Changesets {
	s := &Changesets{revisions: r, changes: c, untracked: u}
	s.worker = newWorker("changeset", change.ErrReloaderFailed, panics, s.reload,
		// A newer reload makes the running one pointless.
		func(change.Gen, change.Gen) bool { return true },
		func(gen change.Gen, err error) change.Update { return change.Update{Gen: gen, Err: err} })
	return s
}

// LoadChangeset resolves the commits and lists the files.
func (s *Changesets) LoadChangeset(ctx context.Context, c change.Comparison) (change.Changeset, error) {
	cs, err := s.load(ctx, c)
	if err != nil {
		return change.Changeset{}, err
	}
	s.mu.Lock()
	s.asked = &c
	s.mu.Unlock()
	return cs, nil
}

// Reload queues a reload of the comparison the first load was asked for.
// It resolves the revisions anew: after a commit or a checkout HEAD names
// another commit, and the diff against the old one would show what is
// committed as changed.
func (s *Changesets) Reload(gen change.Gen) { s.submit(gen) }

func (s *Changesets) reload(ctx context.Context, gen change.Gen) change.Update {
	s.mu.Lock()
	asked := s.asked
	s.mu.Unlock()
	if asked == nil {
		return change.Update{Gen: gen, Err: change.ErrNotLoaded}
	}
	cs, err := s.load(ctx, *asked)
	return change.Update{Gen: gen, Changeset: cs, Err: err}
}

func (s *Changesets) load(ctx context.Context, c change.Comparison) (change.Changeset, error) {
	resolved, err := s.resolve(ctx, c)
	if err != nil {
		return change.Changeset{}, err
	}
	return s.list(ctx, resolved)
}

// list lists the files of a resolved comparison.
func (s *Changesets) list(ctx context.Context, resolved change.Comparison) (change.Changeset, error) {
	files, err := s.changes.ListChanges(ctx, resolved)
	if err != nil {
		return change.Changeset{}, fmt.Errorf("list changes: %w", err)
	}
	if resolved.Right.Kind() == change.KindWorktree {
		paths, err := s.untracked.ListUntracked(ctx)
		if err != nil {
			return change.Changeset{}, fmt.Errorf("list untracked files: %w", err)
		}
		for _, p := range paths {
			files = append(files, change.ChangedFile{Status: change.Untracked, NewPath: p})
		}
	}
	slices.SortStableFunc(files, func(a, b change.ChangedFile) int { return cmp.Compare(a.Path(), b.Path()) })
	return change.Changeset{Comparison: resolved, Files: files}, nil
}

func (s *Changesets) resolve(ctx context.Context, c change.Comparison) (change.Comparison, error) {
	left, err := s.resolveSide(ctx, c.Left)
	if err != nil {
		return change.Comparison{}, err
	}
	right, err := s.resolveSide(ctx, c.Right)
	if err != nil {
		return change.Comparison{}, err
	}
	return change.NewComparison(left, right)
}

func (s *Changesets) resolveSide(ctx context.Context, src change.Source) (change.Source, error) {
	if src.Kind() != change.KindCommit {
		return src, nil
	}
	id, err := s.revisions.ResolveCommit(ctx, src.Rev())
	if err != nil {
		return change.Source{}, err
	}
	return src.Resolved(id)
}
