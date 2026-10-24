// Package lsp talks to language servers over the Language Server Protocol:
// it starts them as child processes, one per server and workspace and at
// most four at a time, tells them about the documents on screen and asks
// them for hovers. It speaks only the part of the protocol seam uses, reads
// no file but the root markers' existence, and ends every process it
// started, along with what those started, when it stops.
package lsp
