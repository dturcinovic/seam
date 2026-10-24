// Package supervise starts every goroutine and timer callback of seam. Each
// Group belongs to one owner, catches panics in its tasks, reports the first
// one to the owner and waits for its tasks on Stop until a deadline.
//
// It does not log by itself and imports nothing but the standard library, so
// use cases, adapters and the TUI may all use it. It does not end goroutines
// that ignore their context (see Group.Stop).
package supervise
