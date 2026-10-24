package filediff_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/change"
	"seam/internal/domain/filediff"
	"seam/internal/domain/hover"
)

func TestReadsWorktree(t *testing.T) {
	commit, err := change.Commit("a").Resolved(mustID(t))
	require.NoError(t, err)
	file := change.ChangedFile{NewPath: "a.go", NewMode: 0o100644}
	deleted := change.ChangedFile{OldPath: "a.go", OldMode: 0o100644}
	tests := []struct {
		name  string
		right change.Source
		file  change.ChangedFile
		want  bool
	}{
		{"worktree", change.Worktree(), file, true},
		{"deleted in the worktree", change.Worktree(), deleted, false},
		{"index", change.Index(), file, false},
		{"commit", commit, file, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := change.NewComparison(commit, tt.right)
			require.NoError(t, err)

			assert.Equal(t, tt.want, filediff.Request{Comparison: c, File: tt.file}.ReadsWorktree())
		})
	}
}

func TestSideNotice(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name    string
		data    []byte
		err     error
		want    *filediff.Notice
		wantErr error
	}{
		{"text", []byte("a\n"), nil, nil, nil},
		{"binary", []byte("a\x00"), nil, &filediff.Notice{Kind: filediff.Binary, Side: filediff.NewSide}, nil},
		{"too large", nil, &change.TooLargeError{Size: 9}, &filediff.Notice{Kind: filediff.TooLarge, Side: filediff.NewSide, Size: 9}, nil},
		{"wrapped too large", nil, errors.Join(boom, &change.TooLargeError{Size: 9}), &filediff.Notice{Kind: filediff.TooLarge, Side: filediff.NewSide, Size: 9}, nil},
		{"other error", nil, boom, nil, boom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := filediff.SideNotice(filediff.NewSide, tt.data, tt.err)

			assert.Equal(t, tt.want, n)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func mustID(t *testing.T) change.ObjectID {
	t.Helper()
	id, err := change.ParseObjectID("0123456789abcdef0123456789abcdef01234567")
	require.NoError(t, err)
	return id
}

func TestDocIsTheDocumentOfASide(t *testing.T) {
	s := filediff.Snapshot{LeftDoc: hover.Document{Path: "old.go"}, RightDoc: hover.Document{Path: "new.go"}}

	assert.Equal(t, "old.go", s.Doc(filediff.OldSide).Path)
	assert.Equal(t, "new.go", s.Doc(filediff.NewSide).Path)
}
