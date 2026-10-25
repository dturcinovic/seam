package in

import "seam/internal/domain/filediff"

//counterfeiter:generate . FileDiffLoader

// FileDiffLoader loads the diff of a file in the background. Load never
// blocks; the outcome waits in Drain until the UI collects it, so a slow
// file never holds up the terminal.
type FileDiffLoader interface {
	// Load asks for a file. It replaces a request still waiting and cancels
	// the running one if that is for another file.
	Load(r filediff.Request)
	// Signal is set, without blocking, after each new update.
	Signal() <-chan struct{}
	// Drain hands over the updates since the last call, oldest first; it
	// computes nothing.
	Drain() []filediff.Update
}

//counterfeiter:generate . StreamLoader

// StreamLoader loads the diffs of the files the stream shows, and of those
// near them, in the background. Like FileDiffLoader it never blocks.
type StreamLoader interface {
	// Want asks for the files of rs in order, the visible ones first. It
	// replaces the files asked for before; the running load goes on if rs
	// asks for the same file in the same generation.
	Want(rs []filediff.Request)
	// Signal is set, without blocking, after each new update.
	Signal() <-chan struct{}
	// Drain hands over the updates since the last call, oldest first; it
	// computes nothing.
	Drain() []filediff.Update
}
