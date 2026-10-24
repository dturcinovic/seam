package usecase

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"seam/internal/app/port/out"
	"seam/internal/domain/change"
)

// Changesets implements in.ChangesetLoader.
type Changesets struct {
	revisions out.RevisionResolver
	changes   out.ChangeLister
	untracked out.UntrackedLister
}

// NewChangesets returns the use case over its ports.
func NewChangesets(r out.RevisionResolver, c out.ChangeLister, u out.UntrackedLister) *Changesets {
	return &Changesets{revisions: r, changes: c, untracked: u}
}

// LoadChangeset resolves the commits once, so a later reload compares
// against the same commits even if a branch has moved.
func (s *Changesets) LoadChangeset(ctx context.Context, c change.Comparison) (change.Changeset, error) {
	resolved, err := s.resolve(ctx, c)
	if err != nil {
		return change.Changeset{}, err
	}
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
