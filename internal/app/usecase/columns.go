package usecase

import (
	"context"

	"github.com/dturcinovic/seam/internal/domain/diff"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/textcell"
)

// columnsEvery is both the content length from which a line gets
// checkpoints and their distance in bytes. Below it, walking a line from its
// start each frame costs too little to be worth an index.
const columnsEvery = 4 << 10

// indexColumns indexes the long lines of both sides of d.
func indexColumns(ctx context.Context, d diff.FileDiff) (filediff.Columns, error) {
	old, err := indexSide(ctx, d.Old)
	if err != nil {
		return filediff.Columns{}, err
	}
	new, err := indexSide(ctx, d.New)
	if err != nil {
		return filediff.Columns{}, err
	}
	return filediff.Columns{Old: old, New: new}, nil
}

func indexSide(ctx context.Context, s diff.Side) (filediff.ColumnIndex, error) {
	var c filediff.ColumnIndex
	for i, line := range s.Lines {
		content := diff.Content(line)
		if len(content) <= columnsEvery {
			continue
		}
		found, err := textcell.Checkpoints(ctx, content, columnsEvery)
		if err != nil {
			return filediff.ColumnIndex{}, err
		}
		points := make([]filediff.Checkpoint, len(found))
		for j, p := range found {
			points[j] = filediff.Checkpoint(p)
		}
		c.Lines = append(c.Lines, filediff.LineCheckpoints{Line: i, Points: points})
	}
	return c, nil
}
