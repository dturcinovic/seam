package change

import (
	"bytes"
	"errors"
	"fmt"
	"time"
)

// Status is how a file changed between the two sides.
type Status int

// The statuses git reports, plus Untracked for files git does not know. The
// zero value is no valid status. Unmerged is a path with a conflict; which
// content it has depends on the side.
const (
	_ Status = iota
	Added
	Deleted
	Modified
	Renamed
	Copied
	TypeChanged
	Unmerged
	Untracked
)

// String is git's one-letter code for the status, "?" for untracked.
func (s Status) String() string {
	switch s {
	case Added:
		return "A"
	case Deleted:
		return "D"
	case Modified:
		return "M"
	case Renamed:
		return "R"
	case Copied:
		return "C"
	case TypeChanged:
		return "T"
	case Unmerged:
		return "U"
	case Untracked:
		return "?"
	}
	return "-"
}

// Mode is a git file mode such as 0o100644. Zero means the side is missing.
type Mode uint32

// Gitlink is the mode of a submodule: its ID names a commit of the
// submodule, not an object of this repository.
const Gitlink Mode = 0o160000

// SubprojectText is the text of a submodule side as git diff shows it: the
// commit it is at, and -dirty if its worktree has changes.
func SubprojectText(commit ObjectID, dirty bool) string {
	text := "Subproject commit " + commit.String()
	if dirty {
		text += "-dirty"
	}
	return text + "\n"
}

// ChangedFile is one file that differs between the sides. Paths are relative
// to the repository root and may hold any byte but NUL. A side that does not
// exist has an empty path, a zero mode and a zero ID; for the worktree side
// the ID may be zero or stale, so the file itself is what counts.
type ChangedFile struct {
	Status           Status
	OldPath, NewPath string
	OldMode, NewMode Mode
	OldID, NewID     ObjectID
	Counts           LineCounts
	// WorktreePath is where the new side of a file on disk lies in the
	// worktree, by which a language server knows it; empty where it lies
	// elsewhere, and for the other sources, which NewPath names already.
	WorktreePath string
}

// LineCounts is how many lines a file gains and loses, as git counts them.
// Known is false where git counts nothing: for a binary file, or one it does
// not track.
type LineCounts struct {
	Added, Deleted int
	Known          bool
}

// Path is the path to show for the file: the new one unless it was deleted.
func (f ChangedFile) Path() string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}

// Changeset is a resolved comparison with the files that differ, ordered by
// Path.
type Changeset struct {
	Comparison Comparison
	Files      []ChangedFile
}

// Stamp identifies the state of a worktree file cheaply: if it is unchanged,
// the content is taken to be unchanged too.
type Stamp struct {
	Size    int64
	ModTime time.Time
	Inode   uint64
}

// Equal reports whether both stamps describe the same state. ModTime is
// compared as an instant: a time.Time read twice need not be == itself.
func (s Stamp) Equal(o Stamp) bool {
	return s.Size == o.Size && s.Inode == o.Inode && s.ModTime.Equal(o.ModTime)
}

// Errors of reading one side of a file.
var (
	// ErrNotExist means the path names nothing.
	ErrNotExist = errors.New("file does not exist")
	// ErrNotRegular means the path names a FIFO, device, socket, directory or
	// an object that is no blob; seam reads none of them.
	ErrNotRegular = errors.New("not a regular file")
)

// TooLargeError means a side exceeds the file size limit. Size is the size
// known when reading stopped, which is a lower bound for a file that grew.
type TooLargeError struct {
	Size int64
}

// Error says how large the side is.
func (e *TooLargeError) Error() string {
	return fmt.Sprintf("file too large: %d bytes", e.Size)
}

// binaryProbe is how much of a file IsBinary looks at, as git does.
const binaryProbe = 8 << 10

// IsBinary reports whether data looks binary: a NUL byte in its first 8 KiB.
func IsBinary(data []byte) bool {
	return bytes.IndexByte(data[:min(len(data), binaryProbe)], 0) >= 0
}
