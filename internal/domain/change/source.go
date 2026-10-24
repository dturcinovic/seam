package change

import (
	"errors"
	"fmt"
	"strings"
)

// Errors of finding what to compare.
var (
	// ErrNoRepository means the directory is not inside a git worktree.
	ErrNoRepository = errors.New("not inside a git worktree")
	// ErrUnknownRevision means a revision names no commit.
	ErrUnknownRevision = errors.New("unknown revision")
)

// ObjectID is a git object name in hex: 40 digits for SHA-1, 64 for SHA-256.
// The zero value means no object, as git's all-zero name does.
type ObjectID struct {
	hex string
}

// ErrInvalidObjectID is returned by ParseObjectID for anything that is not a
// full object name.
var ErrInvalidObjectID = errors.New("invalid object id")

// ParseObjectID checks s and returns it as an ObjectID. git's all-zero name
// yields the zero ObjectID.
func ParseObjectID(s string) (ObjectID, error) {
	if (len(s) != 40 && len(s) != 64) || strings.Trim(s, "0123456789abcdef") != "" {
		return ObjectID{}, fmt.Errorf("%w: %q", ErrInvalidObjectID, s)
	}
	if strings.Trim(s, "0") == "" {
		return ObjectID{}, nil
	}
	return ObjectID{hex: s}, nil
}

// IsZero reports whether id names no object.
func (id ObjectID) IsZero() bool { return id.hex == "" }

// String is the id in hex, empty for the zero id.
func (id ObjectID) String() string { return id.hex }

// SourceKind says where one side of a comparison comes from.
type SourceKind int

// The kinds of source. The zero value is no valid kind.
const (
	_ SourceKind = iota
	KindCommit
	KindIndex
	KindWorktree
)

// Source is one side of a comparison. A commit source starts as the revision
// the user named and is resolved to an ObjectID before files are listed.
type Source struct {
	kind SourceKind
	rev  string
	id   ObjectID
}

// Commit is the commit that rev names, not yet resolved.
func Commit(rev string) Source { return Source{kind: KindCommit, rev: rev} }

// Index is the staging area.
func Index() Source { return Source{kind: KindIndex} }

// Worktree is the files on disk.
func Worktree() Source { return Source{kind: KindWorktree} }

// Kind says where the side comes from.
func (s Source) Kind() SourceKind { return s.kind }

// Rev is the revision the user named; empty unless Kind is KindCommit.
func (s Source) Rev() string { return s.rev }

// ID is the resolved commit; zero until Resolved or unless Kind is
// KindCommit.
func (s Source) ID() ObjectID { return s.id }

// ErrNotACommit is returned by Resolved for a source that is no commit or
// an id that names nothing.
var ErrNotACommit = errors.New("only a commit source resolves to an object")

// Resolved returns the commit source with its object id.
func (s Source) Resolved(id ObjectID) (Source, error) {
	if s.kind != KindCommit || id.IsZero() {
		return Source{}, fmt.Errorf("%w: %s", ErrNotACommit, s)
	}
	s.id = id
	return s, nil
}

// String names the side as the user would: the revision, index or worktree.
func (s Source) String() string {
	switch s.kind {
	case KindCommit:
		return s.rev
	case KindIndex:
		return "index"
	case KindWorktree:
		return "worktree"
	}
	return "unset"
}

// Comparison is the pair of sides seam shows, old on the left.
type Comparison struct {
	Left, Right Source
}

// ErrInvalidComparison is returned for a pair of sides seam cannot compare.
var ErrInvalidComparison = errors.New("invalid comparison")

// NewComparison checks that left is a commit and right is a commit, the
// index or the worktree.
func NewComparison(left, right Source) (Comparison, error) {
	if left.kind != KindCommit || left.rev == "" {
		return Comparison{}, fmt.Errorf("%w: the left side must be a commit", ErrInvalidComparison)
	}
	switch right.kind {
	case KindCommit:
		if right.rev == "" {
			return Comparison{}, fmt.Errorf("%w: the right commit has no revision", ErrInvalidComparison)
		}
	case KindIndex, KindWorktree:
	default:
		return Comparison{}, fmt.Errorf("%w: the right side is unset", ErrInvalidComparison)
	}
	return Comparison{Left: left, Right: right}, nil
}
