package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/supervise"
)

// Config locates the repository and seam's checkouts of it.
type Config struct {
	// Root is the top of the worktree, as FindRoot returns it.
	Root string
	// Checkouts is the directory that holds seam's checkouts of all
	// repositories, each under a name of its own.
	Checkouts string
}

// Repo is the git repository seam compares in. It implements the outgoing
// ports for listing changes and reading both sides of a file.
type Repo struct {
	root  string
	files *os.Root
	tasks *supervise.Group
	// reads watches the reads from cat-file; see readAbortable.
	reads  *supervise.Group
	blobMu sync.Mutex
	blobs  *catFile
	closed bool
}

// ErrClosed is returned by reads after Close.
var ErrClosed = errors.New("git: repository closed")

// FindRoot returns the top of the worktree that holds dir, or an error
// wrapping change.ErrNoRepository.
func FindRoot(ctx context.Context, dir string) (string, error) {
	out, err := run(ctx, dir, "rev-parse", "--show-toplevel")
	if gitErr, ok := errors.AsType[*commandError](err); ok && gitErr.code == 128 &&
		strings.Contains(gitErr.stderr, "not a git repository") {
		return "", fmt.Errorf("%s: %w", dir, change.ErrNoRepository)
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

// Open opens the repository whose worktree starts at root. It runs no git;
// Close releases it.
func Open(root string, panics supervise.PanicFunc) (*Repo, error) {
	files, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open worktree: %w", err)
	}
	return &Repo{
		root: root, files: files,
		tasks: supervise.NewGroup("git", panics, nil),
		reads: supervise.NewGroup("git reads", panics, nil),
	}, nil
}

// Close ends the cat-file process and releases the worktree.
func (r *Repo) Close(ctx context.Context) error {
	// Ends a read in flight before waiting for the blobMu it holds.
	readsErr := r.reads.Stop(ctx)
	r.blobMu.Lock()
	cat := r.blobs
	r.blobs = nil
	// A read after Close would start a process that nothing ends.
	r.closed = true
	r.blobMu.Unlock()
	var err error
	if cat != nil {
		err = cat.close(r.tasks)
	}
	return errors.Join(readsErr, err, r.tasks.Stop(ctx), r.files.Close())
}

// ResolveCommit implements out.RevisionResolver.
func (r *Repo) ResolveCommit(ctx context.Context, rev string) (change.ObjectID, error) {
	out, err := run(ctx, r.root, "rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}")
	// With --verify --quiet, exit code 1 and nothing else means no such
	// commit; any other failure is git's own and passes through.
	if gitErr, ok := errors.AsType[*commandError](err); ok && gitErr.code == 1 {
		return change.ObjectID{}, fmt.Errorf("%w %q", change.ErrUnknownRevision, rev)
	}
	if err != nil {
		return change.ObjectID{}, err
	}
	return change.ParseObjectID(strings.TrimSuffix(string(out), "\n"))
}

// ListChanges implements out.ChangeLister. The commits of c must be resolved.
func (r *Repo) ListChanges(ctx context.Context, c change.Comparison) ([]change.ChangedFile, error) {
	if c.Left.ID().IsZero() {
		return nil, fmt.Errorf("left side %s is not resolved", c.Left)
	}
	args := []string{"diff", "--raw", "--numstat", "-z", "--no-abbrev", "-M", "--no-ext-diff", "--no-color"}
	switch c.Right.Kind() {
	case change.KindWorktree:
	case change.KindIndex:
		args = append(args, "--cached")
	case change.KindCommit:
		if c.Right.ID().IsZero() {
			return nil, fmt.Errorf("right side %s is not resolved", c.Right)
		}
	case change.KindPath:
		return nil, fmt.Errorf("right side %s is on disk, outside git", c.Right)
	default:
		return nil, fmt.Errorf("right side %s cannot be listed", c.Right)
	}
	args = append(args, c.Left.ID().String())
	if c.Right.Kind() == change.KindCommit {
		args = append(args, c.Right.ID().String())
	}
	out, err := run(ctx, r.root, append(args, "--")...)
	if err != nil {
		return nil, err
	}
	return parseDiff(out)
}

// ListUntracked implements out.UntrackedLister. git names an embedded
// repository as one entry with a trailing slash instead of descending into it.
func (r *Repo) ListUntracked(ctx context.Context) ([]string, error) {
	out, err := run(ctx, r.root, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(splitNUL(out), func(path string) bool {
		return strings.HasSuffix(path, "/")
	}), nil
}

// command is the one way seam starts git. --no-optional-locks keeps git from
// taking the index lock for a stat refresh, which a concurrent git command
// of the user would trip over. LC_ALL=C keeps its messages in the one
// language FindRoot can recognise and seam's own messages use.
func command(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-optional-locks", "-C", dir}, args...)...)
	cmd.Env = append(environ(), "LC_ALL=C")
	return cmd
}

// environ is seam's environment without the variables that tie git to one
// repository, as a git hook or alias that runs seam has them set. dir
// alone names the repository: a relative GIT_INDEX_FILE would point a new
// checkout at the index of the directory it is run in.
func environ() []string {
	repo := []string{"GIT_DIR=", "GIT_WORK_TREE=", "GIT_INDEX_FILE=", "GIT_COMMON_DIR=", "GIT_PREFIX="}
	return slices.DeleteFunc(os.Environ(), func(kv string) bool {
		return slices.ContainsFunc(repo, func(p string) bool { return strings.HasPrefix(kv, p) })
	})
}

// commandError is a git command that ran and failed.
type commandError struct {
	args   []string
	code   int
	stderr string
}

func (e *commandError) Error() string {
	return fmt.Sprintf("git %s: exit %d: %s", e.args[0], e.code, reason(e.stderr))
}

// reason picks the line of git's stderr that says why it failed; warnings
// and hints may come first.
func reason(stderr string) string {
	lines := strings.Split(stderr, "\n")
	for _, l := range lines {
		for _, prefix := range []string{"fatal: ", "error: "} {
			if msg, ok := strings.CutPrefix(l, prefix); ok {
				return msg
			}
		}
	}
	return lines[0]
}

func run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return output(ctx, command(ctx, dir, args...), args)
}

// output runs cmd, git with args, and returns its stdout.
func output(ctx context.Context, cmd *exec.Cmd, args []string) ([]byte, error) {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && ctx.Err() == nil {
		return nil, &commandError{args: args, code: exitErr.ExitCode(), stderr: strings.TrimSpace(stderr.String())}
	}
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", args[0], errors.Join(ctx.Err(), err))
	}
	return out, nil
}

func splitNUL(b []byte) []string {
	s := strings.TrimSuffix(string(b), "\x00")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\x00")
}

// parseDiff reads `git diff --raw --numstat -z`: first per file a raw
// header ":oldmode newmode oldid newid status", then one path, or two for a
// rename or copy; after all of them, in the same order, per file its counts
// "added\tdeleted\tpath", or "added\tdeleted\t" and both paths for a
// rename or copy. Every field is NUL-terminated.
func parseDiff(b []byte) ([]change.ChangedFile, error) {
	fields := splitNUL(b)
	var files []change.ChangedFile
	i := 0
	for i < len(fields) && strings.HasPrefix(fields[i], ":") {
		f, n, err := parseEntry(fields[i:])
		if err != nil {
			return nil, fmt.Errorf("diff --raw entry %q: %w", fields[i], err)
		}
		files = append(files, f)
		i += n
	}
	for j := range files {
		if i >= len(fields) {
			return nil, fmt.Errorf("diff --numstat: no counts for %q", files[j].Path())
		}
		var n int
		var err error
		files[j].Counts, n, err = parseCounts(fields[i:], files[j])
		if err != nil {
			return nil, fmt.Errorf("diff --numstat entry %q: %w", fields[i], err)
		}
		i += n
	}
	if i < len(fields) {
		return nil, fmt.Errorf("diff --numstat: unexpected %q", fields[i])
	}
	return files, nil
}

// parseCounts reads the counts of f and says how many fields they took.
// git writes "-" for both counts of a binary file.
func parseCounts(fields []string, f change.ChangedFile) (change.LineCounts, int, error) {
	parts := strings.SplitN(fields[0], "\t", 3)
	if len(parts) != 3 {
		return change.LineCounts{}, 0, errors.New("unexpected counts")
	}
	n, paths := 1, []string{parts[2]}
	want := []string{f.Path()}
	if parts[2] == "" {
		if len(fields) < 3 {
			return change.LineCounts{}, 0, errors.New("path missing")
		}
		n, paths, want = 3, fields[1:3], []string{f.OldPath, f.NewPath}
	}
	if !slices.Equal(paths, want) {
		return change.LineCounts{}, 0, fmt.Errorf("counts for %q, not for %q", paths, want)
	}
	if parts[0] == "-" && parts[1] == "-" {
		return change.LineCounts{}, n, nil
	}
	// One bit short of int, so the conversion below cannot wrap.
	added, errA := strconv.ParseUint(parts[0], 10, strconv.IntSize-1)
	deleted, errD := strconv.ParseUint(parts[1], 10, strconv.IntSize-1)
	if err := errors.Join(errA, errD); err != nil {
		return change.LineCounts{}, 0, fmt.Errorf("bad counts: %w", err)
	}
	return change.LineCounts{Added: int(added), Deleted: int(deleted), Known: true}, n, nil
}

func parseEntry(fields []string) (change.ChangedFile, int, error) {
	rest, ok := strings.CutPrefix(fields[0], ":")
	header := strings.Fields(rest)
	if !ok || len(header) != 5 || header[4] == "" {
		return change.ChangedFile{}, 0, errors.New("unexpected header")
	}
	status, paths, err := parseStatus(header[4][0])
	if err != nil {
		return change.ChangedFile{}, 0, err
	}
	if len(fields) < 1+paths {
		return change.ChangedFile{}, 0, errors.New("path missing")
	}
	f := change.ChangedFile{Status: status, OldPath: fields[1], NewPath: fields[paths]}
	if f.OldMode, err = parseMode(header[0]); err != nil {
		return change.ChangedFile{}, 0, err
	}
	if f.NewMode, err = parseMode(header[1]); err != nil {
		return change.ChangedFile{}, 0, err
	}
	if f.OldID, err = change.ParseObjectID(header[2]); err != nil {
		return change.ChangedFile{}, 0, err
	}
	if f.NewID, err = change.ParseObjectID(header[3]); err != nil {
		return change.ChangedFile{}, 0, err
	}
	// Mode 000000 marks a side that does not exist: added, deleted, or the
	// index side of a conflict.
	if f.OldMode == 0 {
		f.OldPath = ""
	}
	if f.NewMode == 0 {
		f.NewPath = ""
	}
	return f, 1 + paths, nil
}

// parseStatus maps git's status letter and says how many paths follow.
func parseStatus(letter byte) (change.Status, int, error) {
	switch letter {
	case 'A':
		return change.Added, 1, nil
	case 'D':
		return change.Deleted, 1, nil
	case 'M':
		return change.Modified, 1, nil
	case 'T':
		return change.TypeChanged, 1, nil
	case 'U':
		return change.Unmerged, 1, nil
	case 'R':
		return change.Renamed, 2, nil
	case 'C':
		return change.Copied, 2, nil
	}
	return 0, 0, fmt.Errorf("unknown status %q", letter)
}

func parseMode(s string) (change.Mode, error) {
	m, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("mode %q: %w", s, err)
	}
	return change.Mode(m), nil
}
