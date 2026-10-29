// Package release holds what seam knows about its own releases: a release
// version, the last check for a newer one and the notice that names it,
// the kinds of install and how each is upgraded, and the archive of a
// release with its checksums. It does not ask anyone, nor tell from where
// seam lies how it was installed; the use case and the composition root
// do.
package release
