package in

import "seam/internal/domain/filediff"

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
