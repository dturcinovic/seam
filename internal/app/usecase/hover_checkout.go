package usecase

import (
	"context"

	"seam/internal/domain/change"
	"seam/internal/domain/hover"
)

// checkoutLocked is how far the checkout of k's commit has come, starting
// it if none was asked for yet. Only a document shown, and so one whose
// language has a server, may start one: a checkout costs the whole tree.
// Zero means the document is not shown.
func (h *Hovers) checkoutLocked(k hover.DocKey) hover.ServerStatus {
	id := k.Commit
	if st, ok := h.trees[id]; ok {
		return st
	}
	if _, shown := h.waiting.get(k); !shown {
		return 0
	}
	h.setTreeLocked(id, hover.Starting)
	err := h.checkingOut.Go("checkout "+id.String(), func(ctx context.Context) {
		dir, err := h.checkouts.Checkout(ctx, id)
		h.checkedOut(id, dir, err)
	})
	if err != nil {
		// Stopping: no checkout comes any more.
		h.setTreeLocked(id, hover.ServerFailed)
	}
	return h.trees[id]
}

// checkedOut takes the outcome of the checkout of id. Once it is there,
// the documents of id waiting for it go to their servers, which start for
// them.
func (h *Hovers) checkedOut(id change.ObjectID, dir string, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err != nil {
		h.setTreeLocked(id, hover.ServerFailed)
		h.dropWaitingLocked(id)
		return
	}
	h.roots.Mount(id, dir)
	for _, d := range h.dropWaitingLocked(id) {
		// The viewer learns the instance from Track once it hears of the
		// checkout.
		h.syncLocked(d)
	}
	h.setTreeLocked(id, hover.Ready)
}

// dropWaitingLocked takes the documents of id out of those waiting.
func (h *Hovers) dropWaitingLocked(id change.ObjectID) []hover.Document {
	var docs []hover.Document
	h.waiting.each(func(k hover.DocKey, d hover.Document) {
		if k.Commit == id {
			docs = append(docs, d)
		}
	})
	for _, d := range docs {
		h.waiting.remove(d.Key())
	}
	return docs
}

// setTreeLocked records and reports how far the checkout of id has come.
// Under the lock, so that the reports come in the order of the states.
func (h *Hovers) setTreeLocked(id change.ObjectID, st hover.ServerStatus) {
	h.trees[id] = st
	h.colours.publish(hover.Update{Kind: hover.CheckoutChanged, Commit: id, Status: st})
}
