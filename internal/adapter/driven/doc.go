// Package driven groups the adapters that implement outgoing ports, one
// subpackage per outside system (git, chroma, the language servers, the
// configuration file). An adapter never imports another adapter.
package driven
