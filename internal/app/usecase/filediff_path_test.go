package usecase_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/limits"
)

// onDisk lets the disk fake read contents by path, stamped by their size;
// anything else is missing.
func (f fileFakes) onDisk(contents map[string]string) {
	var mu sync.Mutex
	stamp := func(path string) (change.Stamp, error) {
		text, ok := contents[path]
		if !ok {
			return change.Stamp{}, change.ErrNotExist
		}
		return change.Stamp{Size: int64(len(text))}, nil
	}
	f.disk.ReadPathCalls(func(_ context.Context, path string, _ limits.FileSize) ([]byte, change.Stamp, error) {
		mu.Lock()
		defer mu.Unlock()
		s, err := stamp(path)
		return []byte(contents[path]), s, err
	})
	f.disk.StatPathCalls(func(_ context.Context, path string) (change.Stamp, error) {
		mu.Lock()
		defer mu.Unlock()
		return stamp(path)
	})
}

func TestFilesOfDirectoriesOnDiskAreReadBelowThem(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	f.onDisk(map[string]string{"/l/sub/a.go": "package a\n", "/r/sub/a.go": "package b\n"})
	file := change.ChangedFile{Status: change.Modified, OldPath: "sub/a.go", NewPath: "sub/a.go"}

	u := loadOne(t, uc, comparison(t, change.Dir("/l"), change.Dir("/r")), 1, file, false)

	require.NoError(t, u.Err)
	assert.Equal(t, []string{"package a|package b"}, rendered(u.Snapshot))
	assert.Zero(t, f.blobs.ReadBlobCallCount()+f.worktree.ReadWorktreeCallCount(), "git reads nothing")
	assert.Equal(t, hover.Document{}, u.Snapshot.LeftDoc, "a file on disk has no commit to ask in")
	assert.Equal(t, hover.Document{}, u.Snapshot.RightDoc, "and this one lies in no worktree")
	assert.Equal(t, "/r/sub/a.go", f.locator.LocateInWorktreeArgsForCall(0))
}

func TestAFileOnDiskInTheWorktreeIsADocument(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	f.onDisk(map[string]string{"/tmp/x_a.go": "package a\n", "a.go": "package b\n"})
	f.locator.LocateInWorktreeReturns("pkg/a.go", true)
	file := change.ChangedFile{Status: change.Modified, OldPath: "a.go", NewPath: "a.go"}

	u := loadOne(t, uc, comparison(t, change.File("/tmp/x_a.go"), change.File("a.go")), 1, file, false)

	require.NoError(t, u.Err)
	assert.Equal(t, hover.Document{}, u.Snapshot.LeftDoc)
	assert.Equal(t, "pkg/a.go", u.Snapshot.RightDoc.Path, "the server knows it by its place in the worktree")
	assert.True(t, u.Snapshot.RightDoc.Commit.IsZero())
	assert.Equal(t, "package b\n", u.Snapshot.RightDoc.Text)
	assert.Equal(t, "go", string(u.Snapshot.RightDoc.Lang))
}

func TestAMissingFileOnDiskIsAnEmptySide(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	f.onDisk(map[string]string{"a.go": "package b\n"})

	u := loadOne(t, uc, comparison(t, change.File(""), change.File("a.go")), 1, change.ChangedFile{Status: change.Added, NewPath: "a.go"}, false)

	require.NoError(t, u.Err)
	assert.Equal(t, []string{"|package b"}, rendered(u.Snapshot))
	assert.Equal(t, 1, f.disk.ReadPathCallCount(), "only the side that exists is read")
}

func TestAChangedFileOnDiskIsNotServedFromTheCache(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	contents := map[string]string{"/l/a.go": "a\n", "/r/a.go": "b\n"}
	f.onDisk(contents)
	c := comparison(t, change.Dir("/l"), change.Dir("/r"))
	file := change.ChangedFile{Status: change.Modified, OldPath: "a.go", NewPath: "a.go"}
	require.NoError(t, loadOne(t, uc, c, 1, file, false).Err)

	again := loadOne(t, uc, c, 2, file, false)
	assert.Equal(t, 2, f.disk.ReadPathCallCount(), "unchanged: from the cache")

	f.onDisk(map[string]string{"/l/a.go": "a\n", "/r/a.go": "bc\n"})
	edited := loadOne(t, uc, c, 3, file, false)

	assert.Equal(t, []string{"a|b"}, rendered(again.Snapshot))
	assert.Equal(t, []string{"a|bc"}, rendered(edited.Snapshot), "edited through the link git difftool -d made")
}
