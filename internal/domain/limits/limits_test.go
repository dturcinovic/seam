package limits_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/limits"
)

func TestNewFileSize(t *testing.T) {
	size, err := limits.NewFileSize(1024)
	require.NoError(t, err)
	assert.Equal(t, int64(1024), size.Bytes())

	for _, n := range []int64{0, -1} {
		_, err := limits.NewFileSize(n)
		assert.ErrorIs(t, err, limits.ErrInvalidFileSize, "n=%d", n)
	}
}
