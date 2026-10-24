package out

import (
	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
)

//counterfeiter:generate . DocumentSync

// DocumentSync tells the language servers which documents are open and
// with what text. Neither call blocks or is refused; the text reaches the
// server once it can take it, the newest version only.
type DocumentSync interface {
	// Sync opens d or brings it to d's version, starting its server if need
	// be. It names the instance that serves d; ok is false if no server
	// hovers over d's language.
	Sync(d hover.Document) (i lang.Instance, ok bool)
	// Close closes the document at path.
	Close(path string)
}

//counterfeiter:generate . HoverRequester

// HoverRequester asks for hovers. Neither call blocks; the answer comes
// through ServerEvents.
type HoverRequester interface {
	Request(p hover.Position) hover.Submit
	// Cancel drops the request: no answer comes for it.
	Cancel(id hover.ReqID)
}

//counterfeiter:generate . ServerEvents

// ServerEvents hands out the answers and status changes of the servers.
// Signal is set after each new one; Drain returns them, oldest first.
type ServerEvents interface {
	Signal() <-chan struct{}
	Drain() []hover.Update
}
