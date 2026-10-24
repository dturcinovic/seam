package in

import (
	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
)

// Hoverer asks the language server about a position. Neither call blocks;
// the answer comes through LSPUpdates.
type Hoverer interface {
	// Hover asks at p. Only the answer to the last p asked is handed on.
	Hover(p hover.Position) hover.Submit
	// CancelHover drops the request: no answer comes for it.
	CancelHover(id hover.ReqID)
}

//counterfeiter:generate . DocumentTracker

// DocumentTracker tells the language servers which documents the viewer
// shows. Neither call blocks or is refused.
type DocumentTracker interface {
	// Track opens d or brings it to d's version, and keeps it open while it
	// is among the last shown. It names the instance that serves d; ok is
	// false if none does, as for a document without a path.
	Track(d hover.Document) (i lang.Instance, ok bool)
	// Untrack closes the document at path: the file is gone, or its text
	// no longer what the viewer shows, and the server reads it from disk.
	Untrack(path string)
}

//counterfeiter:generate . LSPUpdates

// LSPUpdates hands out the answers and status changes of the servers.
type LSPUpdates interface {
	// Signal is set, without blocking, after each new update.
	Signal() <-chan struct{}
	// Drain hands over the updates since the last call, oldest first; it
	// computes nothing.
	Drain() []hover.Update
}
