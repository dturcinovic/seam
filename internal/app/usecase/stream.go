package usecase

import (
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/supervise"
)

// Streams implements in.StreamLoader. It loads one file after another on
// a worker of its own, through the reading and the cache of FileDiffs: the
// single file and the stream share what is loaded.
type Streams struct {
	*worker[filediff.Request, filediff.Update]
}

// NewStreams returns the stream loader over f. Its worker runs between
// Start and Stop.
func NewStreams(f *FileDiffs, panics supervise.PanicFunc) *Streams {
	return &Streams{newWorker("stream", filediff.ErrLoaderFailed, panics, f.load, supersedes, failedLoad)}
}

// Want queues rs; it does no I/O, not even for a cached file.
func (s *Streams) Want(rs []filediff.Request) { s.want(rs, sameLoad) }

// sameLoad reports that r asks for what is running: a generation fixes the
// comparison, and a fresh read is asked once per generation.
func sameLoad(running, r filediff.Request) bool {
	return running.Gen == r.Gen && running.File == r.File
}
