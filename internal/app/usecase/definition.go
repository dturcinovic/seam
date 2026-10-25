package usecase

import "seam/internal/domain/hover"

// Define asks where the symbol at p is defined, as Hover asks about it.
func (h *Hovers) Define(p hover.Position) hover.Submit {
	if sub := h.admit(p, &h.latestDefinition); sub != hover.Sent {
		return sub
	}
	return h.definitions.RequestDefinition(p)
}

// CancelDefinition drops the request id.
func (h *Hovers) CancelDefinition(id hover.ReqID) {
	h.definitions.CancelDefinition(id)
}
