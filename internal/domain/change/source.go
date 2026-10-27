package change

import (
	"errors"
	"fmt"
	"path/filepath"
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
	KindPath
)

// Source is one side of a comparison. A commit source starts as the revision
// the user named and is resolved to an ObjectID before files are listed.
type Source struct {
	kind SourceKind
	rev  string
	id   ObjectID
	path string
	dir  bool
}

// Commit is the commit that rev names, not yet resolved.
func Commit(rev string) Source { return Source{kind: KindCommit, rev: rev} }

// Index is the staging area.
func Index() Source { return Source{kind: KindIndex} }

// Worktree is the files on disk.
func Worktree() Source { return Source{kind: KindWorktree} }

// File is a file on disk, outside git's view. An empty path is a side that
// does not exist, as git difftool passes /dev/null for it.
func File(path string) Source { return Source{kind: KindPath, path: path} }

// Dir is a directory on disk, outside git's view.
func Dir(path string) Source { return Source{kind: KindPath, path: path, dir: true} }

// Kind says where the side comes from.
func (s Source) Kind() SourceKind { return s.kind }

// Rev is the revision the user named; empty unless Kind is KindCommit.
func (s Source) Rev() string { return s.rev }

// ID is the resolved commit; zero until Resolved or unless Kind is
// KindCommit.
func (s Source) ID() ObjectID { return s.id }

// Path is the file or directory the user named; empty unless Kind is
// KindPath.
func (s Source) Path() string { return s.path }

// IsDir reports whether the side is a directory on disk.
func (s Source) IsDir() bool { return s.dir }

// At is where the file rel of a path side lies: below a directory, or the
// file side itself.
func (s Source) At(rel string) string {
	if s.dir {
		return filepath.Join(s.path, rel)
	}
	return s.path
}

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
	case KindPath:
		if s.path == "" {
			return "/dev/null"
		}
		return s.path
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
// index or the worktree, or that both are files or both directories on disk.
func NewComparison(left, right Source) (Comparison, error) {
	if left.kind == KindPath || right.kind == KindPath {
		return pathComparison(left, right)
	}
	if left.kind != KindCommit || left.rev == "" {
		return Comparison{}, fmt.Errorf("%w: the left side must be a commit", ErrInvalidComparison)
	}
	switch right.kind {
	case KindCommit:
		if right.rev == "" {
			return Comparison{}, fmt.Errorf("%w: the right commit has no revision", ErrInvalidComparison)
		}
	case KindIndex, KindWorktree:
	case KindPath: // pathComparison took it.
	default:
		return Comparison{}, fmt.Errorf("%w: the right side is unset", ErrInvalidComparison)
	}
	return Comparison{Left: left, Right: right}, nil
}

func pathComparison(left, right Source) (Comparison, error) {
	switch {
	case left.kind != KindPath || right.kind != KindPath:
		return Comparison{}, fmt.Errorf("%w: a file on disk compares only with another", ErrInvalidComparison)
	case left.dir != right.dir:
		return Comparison{}, fmt.Errorf("%w: %s and %s are a file and a directory", ErrInvalidComparison, left, right)
	case left.dir && (left.path == "" || right.path == ""):
		return Comparison{}, fmt.Errorf("%w: a directory needs a path", ErrInvalidComparison)
	case left.path == "" && right.path == "":
		return Comparison{}, fmt.Errorf("%w: both sides are missing", ErrInvalidComparison)
	}
	return Comparison{Left: left, Right: right}, nil
}
