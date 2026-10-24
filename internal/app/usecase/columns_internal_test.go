package usecase

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"seam/internal/domain/diff"
)

func TestIndexingStopsWhenTheJobIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	long := strings.Repeat("a", 3*columnsEvery)

	for _, d := range []diff.FileDiff{
		{Old: diff.Split(long)},
		{New: diff.Split(long)},
	} {
		_, err := indexColumns(ctx, d)
		require.ErrorIs(t, err, context.Canceled)
	}
}
