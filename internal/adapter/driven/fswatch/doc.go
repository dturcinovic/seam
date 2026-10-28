// Package fswatch watches a repository's files with fsnotify (inotify on
// Linux, kqueue on macOS) and reports that something changed, never what.
// It does not decide what git ignores, which the git adapter knows, nor
// when a burst of changes has settled, which is the use case's to judge.
package fswatch
