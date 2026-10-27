package change_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/change"
)

func TestBlobIDIsGitsName(t *testing.T) {
	// As git hash-object names them.
	assert.Equal(t, "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391", change.BlobID(change.SHA1, nil).String())
	assert.Equal(t, "ce013625030ba8dba906f756967f9e9ca394464a", change.BlobID(change.SHA1, []byte("hello\n")).String())
	assert.Equal(t, "473a0f4c3be8a93681a267e3b1e9a7dcda1185436fe141f7749120a303721813", change.BlobID(change.SHA256, nil).String())
	assert.True(t, change.BlobID(0, []byte("x")).IsZero())

	f, err := change.ParseObjectFormat("sha256")
	require.NoError(t, err)
	assert.Equal(t, change.SHA256, f)
	_, err = change.ParseObjectFormat("md5")
	require.ErrorIs(t, err, change.ErrUnknownObjectFormat)
}
