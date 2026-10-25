package out

import (
	"context"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/lang"
)

//counterfeiter:generate . DocumentSync

// DocumentSync tells the language servers which documents are open and
// with what text. Neither call blocks or is refused; the text reaches the
// server once it can take it, the newest version only.
type DocumentSync interface {
	// Sync opens d or brings it to d's version, starting its server if need
	// be. It names the instance that serves d; ok is false if no server
	// hovers over d's language, or d's commit has no checkout mounted.
	Sync(d hover.Document) (i lang.Instance, ok bool)
	// Close closes the document k; its server forgets the text.
	Close(k hover.DocKey)
}

//counterfeiter:generate . CheckoutRoots

// CheckoutRoots tells the language servers where the checkout of a commit
// lies, so documents of that commit can be served from it.
type CheckoutRoots interface {
	// Mount makes dir the tree of commit id's documents.
	Mount(id change.ObjectID, dir string)
}

//counterfeiter:generate . CommitCheckouts

// CommitCheckouts makes checkouts of commits for the session, which go
// with it.
type CommitCheckouts interface {
	// Checkout returns the directory of a checkout of commit id, making it
	// the first time. That takes as long as git takes to write the tree.
	Checkout(ctx context.Context, id change.ObjectID) (dir string, err error)
}

//counterfeiter:generate . HoverRequester

// HoverRequester asks for hovers. Neither call blocks; the answer comes
// through ServerEvents.
type HoverRequester interface {
	Request(p hover.Position) hover.Submit
	// Cancel drops the request: no answer comes for it.
	Cancel(id hover.ReqID)
}

//counterfeiter:generate . DefinitionRequester

// DefinitionRequester asks where a symbol is defined. Neither call blocks;
// the answer comes through ServerEvents as DefinitionDone.
type DefinitionRequester interface {
	RequestDefinition(p hover.Position) hover.Submit
	// CancelDefinition drops the request: no answer comes for it.
	CancelDefinition(id hover.ReqID)
}

//counterfeiter:generate . ServerEvents

// ServerEvents hands out the answers and status changes of the servers.
// Signal is set after each new one; Drain returns them, oldest first.
type ServerEvents interface {
	Signal() <-chan struct{}
	Drain() []hover.Update
}
