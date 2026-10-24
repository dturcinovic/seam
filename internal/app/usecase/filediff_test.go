package usecase_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/app/port/out/outfakes"
	"seam/internal/app/usecase"
	"seam/internal/domain/change"
	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/limits"
	"seam/internal/testsupport"
)

// repo is the content behind the fakes: blobs by id, worktree files by path.
type repo struct {
	mu       sync.Mutex
	blobs    map[change.ObjectID]string
	worktree map[string]string
	stamps   map[string]change.Stamp
}

func (r *repo) readWorktree(path string) ([]byte, change.Stamp, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return []byte(r.worktree[path]), r.stamps[path], nil
}

type fileFakes struct {
	blobs    *outfakes.FakeBlobReader
	worktree *outfakes.FakeWorktreeReader
	stats    *outfakes.FakeWorktreeStatter
	repo     *repo
}

func newFileDiffs(t *testing.T) (*usecase.FileDiffs, fileFakes) {
	t.Helper()
	r := &repo{blobs: map[change.ObjectID]string{}, worktree: map[string]string{}, stamps: map[string]change.Stamp{}}
	f := fileFakes{&outfakes.FakeBlobReader{}, &outfakes.FakeWorktreeReader{}, &outfakes.FakeWorktreeStatter{}, r}
	f.blobs.ReadBlobCalls(func(_ context.Context, id change.ObjectID, _ limits.FileSize) ([]byte, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		return []byte(r.blobs[id]), nil
	})
	f.worktree.ReadWorktreeCalls(func(_ context.Context, path string, _ limits.FileSize) ([]byte, change.Stamp, error) {
		return r.readWorktree(path)
	})
	f.stats.StatWorktreeCalls(func(_ context.Context, path string) (change.Stamp, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.stamps[path], nil
	})
	return usecase.NewFileDiffs(f.blobs, f.worktree, f.stats, limits.DefaultFileSize, nil), f
}

// start runs the worker for the test.
func start(t *testing.T, uc interface {
	Start() error
	Stop(context.Context) error
},
) {
	t.Helper()
	require.NoError(t, uc.Start())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), testsupport.Guard)
		defer cancel()
		assert.NoError(t, uc.Stop(ctx))
	})
}

func id(t *testing.T, name string, side byte) change.ObjectID {
	t.Helper()
	return mustID(t, fmt.Sprintf("%040x", fmt.Sprintf("%s%c", name, side))[:40])
}

// modified is a changed file whose sides have the given contents: old as a
// blob, new as a blob and in the worktree, so any comparison finds it.
func (f fileFakes) modified(t *testing.T, path, old, new string) change.ChangedFile {
	t.Helper()
	file := change.ChangedFile{
		Status: change.Modified, OldPath: path, NewPath: path,
		OldMode: 0o100644, NewMode: 0o100644, OldID: id(t, path, 'o'), NewID: id(t, path, 'n'),
	}
	f.repo.mu.Lock()
	defer f.repo.mu.Unlock()
	f.repo.blobs[file.OldID], f.repo.blobs[file.NewID] = old, new
	f.repo.worktree[path] = new
	f.repo.stamps[path] = change.Stamp{Size: int64(len(new)), ModTime: time.Unix(1, 0), Inode: 7}
	return file
}

// edit changes the worktree file; touched says whether its stamp changes.
func (f fileFakes) edit(path, content string, touched bool) {
	f.repo.mu.Lock()
	defer f.repo.mu.Unlock()
	f.repo.worktree[path] = content
	if touched {
		s := f.repo.stamps[path]
		s.ModTime = s.ModTime.Add(time.Second)
		f.repo.stamps[path] = s
	}
}

func worktreeComparison(t *testing.T) change.Comparison {
	return comparison(t, mustSource(t, "a"), change.Worktree())
}

func mustSource(t *testing.T, rev string) change.Source {
	t.Helper()
	s, err := change.Commit(rev).Resolved(mustID(t, strings.Repeat("a", 40)))
	require.NoError(t, err)
	return s
}

// loadOne loads file and waits for the update of gen.
func loadOne(t *testing.T, uc *usecase.FileDiffs, c change.Comparison, gen filediff.Gen, file change.ChangedFile, fresh bool) filediff.Update {
	t.Helper()
	uc.Load(filediff.Request{Gen: gen, Comparison: c, File: file, Fresh: fresh})
	all := testsupport.Collect(t, uc, func(u []filediff.Update) bool { return len(u) > 0 && u[len(u)-1].Gen == gen })
	return all[len(all)-1]
}

func rendered(s filediff.Snapshot) []string {
	var out []string
	for _, r := range s.Diff.Rows {
		left, right := "", ""
		if r.Left >= 0 {
			left = diff.Content(s.Diff.Old.Lines[r.Left])
		}
		if r.Right >= 0 {
			right = diff.Content(s.Diff.New.Lines[r.Right])
		}
		out = append(out, left+"|"+right)
	}
	return out
}

func TestLoadReadsEachSideFromItsSource(t *testing.T) {
	tests := []struct {
		name         string
		right        func(*testing.T) change.Source
		wantWorktree int
	}{
		{"worktree", func(*testing.T) change.Source { return change.Worktree() }, 1},
		{"index", func(*testing.T) change.Source { return change.Index() }, 0},
		{"commit", func(t *testing.T) change.Source { return mustSource(t, "b") }, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc, f := newFileDiffs(t)
			start(t, uc)
			file := f.modified(t, "a.go", "x\ny\n", "x\nz\n")

			u := loadOne(t, uc, comparison(t, mustSource(t, "a"), tt.right(t)), 1, file, false)

			require.NoError(t, u.Err)
			assert.Equal(t, []string{"x|x", "y|z"}, rendered(u.Snapshot))
			assert.Equal(t, file, u.Snapshot.File)
			assert.Equal(t, filediff.Version(1), u.Snapshot.Version)
			assert.Empty(t, u.Snapshot.Notices)
			assert.Equal(t, tt.wantWorktree, f.worktree.ReadWorktreeCallCount())
			_, oldID, max := f.blobs.ReadBlobArgsForCall(0)
			assert.Equal(t, file.OldID, oldID)
			assert.Equal(t, limits.DefaultFileSize, max, "the readers get the limit")
			if tt.wantWorktree == 0 {
				_, newID, _ := f.blobs.ReadBlobArgsForCall(1)
				assert.Equal(t, file.NewID, newID)
			}
		})
	}
}

func TestMissingSidesAreEmpty(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	file := f.modified(t, "a.go", "x\n", "y\n")
	added, deleted := file, file
	added.Status, added.OldPath, added.OldMode, added.OldID = change.Added, "", 0, change.ObjectID{}
	deleted.Status, deleted.NewPath, deleted.NewMode, deleted.NewID = change.Deleted, "", 0, change.ObjectID{}
	c := worktreeComparison(t)

	u := loadOne(t, uc, c, 1, added, false)
	assert.Equal(t, []string{"|y"}, rendered(u.Snapshot))
	u = loadOne(t, uc, c, 2, deleted, false)
	assert.Equal(t, []string{"x|"}, rendered(u.Snapshot))
	assert.Zero(t, u.Snapshot.Version, "no text on the right")
	assert.Equal(t, 1, f.worktree.ReadWorktreeCallCount(), "a deleted file is not read")
}

func TestAnUntrackedFileIsReadFromTheWorktree(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	f.modified(t, "new.go", "", "n\n")
	untracked := change.ChangedFile{Status: change.Untracked, NewPath: "new.go"}
	c := worktreeComparison(t)

	u := loadOne(t, uc, c, 1, untracked, false)

	require.NoError(t, u.Err)
	assert.Equal(t, []string{"|n"}, rendered(u.Snapshot))
	assert.Equal(t, filediff.Version(1), u.Snapshot.Version)
	assert.Zero(t, f.blobs.ReadBlobCallCount(), "no old side")
	f.edit("new.go", "m\n", true)
	assert.Equal(t, []string{"|m"}, rendered(loadOne(t, uc, c, 2, untracked, false).Snapshot), "its stamp is checked")
}

func TestAVersionNeverGoesBack(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	staged := f.modified(t, "a.go", "x\n", "X\n")
	restaged := staged
	restaged.NewID = id(t, "a.go", 'm')
	f.repo.blobs[restaged.NewID] = "Y\n"
	c := comparison(t, mustSource(t, "a"), change.Index())

	assert.Equal(t, filediff.Version(1), loadOne(t, uc, c, 1, staged, false).Snapshot.Version)
	assert.Equal(t, filediff.Version(2), loadOne(t, uc, c, 2, restaged, false).Snapshot.Version)
	// Staging X again brings back the first key, which is still cached.
	u := loadOne(t, uc, c, 3, staged, false)
	assert.Equal(t, 4, f.blobs.ReadBlobCallCount(), "the third load is a hit")
	assert.Equal(t, filediff.Version(3), u.Snapshot.Version)
}

func TestTooLargeAndBinarySidesBecomeNotices(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	file := f.modified(t, "a.go", "x\n", "bin\x00ary")
	f.blobs.ReadBlobReturnsOnCall(0, nil, &change.TooLargeError{Size: 9 << 20})
	f.blobs.ReadBlobCalls(nil)

	u := loadOne(t, uc, worktreeComparison(t), 1, file, false)

	require.NoError(t, u.Err)
	assert.Equal(t, []filediff.Notice{
		{Kind: filediff.TooLarge, Side: filediff.OldSide, Size: 9 << 20},
		{Kind: filediff.Binary, Side: filediff.NewSide},
	}, u.Snapshot.Notices)
	assert.Empty(t, u.Snapshot.Diff.Rows)
	assert.Zero(t, u.Snapshot.Version)
}

func TestTextThatIsNotShownHasNoVersion(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	file := f.modified(t, "a.go", "bin\x00ary", "y\n")

	u := loadOne(t, uc, worktreeComparison(t), 1, file, false)

	assert.Equal(t, []filediff.Notice{{Kind: filediff.Binary, Side: filediff.OldSide}}, u.Snapshot.Notices)
	assert.Zero(t, u.Snapshot.Version)
}

func TestATooLargeWorktreeFileStaysCached(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	file := f.modified(t, "a.go", "x\n", "y\n")
	f.worktree.ReadWorktreeCalls(nil)
	f.worktree.ReadWorktreeReturns(nil, change.Stamp{}, &change.TooLargeError{Size: 9 << 20})
	c := worktreeComparison(t)

	loadOne(t, uc, c, 1, file, false)
	loadOne(t, uc, c, 2, file, false)

	assert.Equal(t, 1, f.worktree.ReadWorktreeCallCount())
	assert.Equal(t, 1, f.blobs.ReadBlobCallCount(), "the old side is not read again")
}

func TestReadErrorsFailTheLoad(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	file := f.modified(t, "a.go", "x\n", "y\n")
	f.worktree.ReadWorktreeCalls(nil)
	f.worktree.ReadWorktreeReturns(nil, change.Stamp{}, change.ErrNotExist)

	u := loadOne(t, uc, worktreeComparison(t), 1, file, false)

	require.ErrorIs(t, u.Err, change.ErrNotExist)
	assert.Equal(t, filediff.Gen(1), u.Gen)
}

func TestTheCacheChecksTheWorktreeStamp(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	file := f.modified(t, "a.go", "x\n", "y\n")
	c := worktreeComparison(t)
	loadOne(t, uc, c, 1, file, false)

	u := loadOne(t, uc, c, 2, file, false)
	assert.Equal(t, 1, f.worktree.ReadWorktreeCallCount(), "an unchanged stamp is a hit")
	assert.Equal(t, filediff.Gen(2), u.Gen, "a hit carries the new generation")

	f.edit("a.go", "z\n", true)
	u = loadOne(t, uc, c, 3, file, false)
	assert.Equal(t, 2, f.worktree.ReadWorktreeCallCount(), "a new stamp loads again")
	assert.Equal(t, []string{"x|z"}, rendered(u.Snapshot))
	assert.Equal(t, filediff.Version(2), u.Snapshot.Version)
}

func TestFreshSkipsTheCacheAndVersionsFollowTheText(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	file := f.modified(t, "a.go", "x\n", "y\n")
	c := worktreeComparison(t)
	loadOne(t, uc, c, 1, file, false)

	f.edit("a.go", "z\n", false)
	u := loadOne(t, uc, c, 2, file, false)
	assert.Equal(t, []string{"x|y"}, rendered(u.Snapshot), "without Fresh the same stamp is trusted")

	u = loadOne(t, uc, c, 3, file, true)
	assert.Equal(t, []string{"x|z"}, rendered(u.Snapshot), "Fresh reads the new text")
	assert.Equal(t, filediff.Version(2), u.Snapshot.Version)

	u = loadOne(t, uc, c, 4, file, true)
	assert.Equal(t, filediff.Version(2), u.Snapshot.Version, "the same text keeps its version")
}
