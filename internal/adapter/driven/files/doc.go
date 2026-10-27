// Package files reads files and directories on disk that git does not
// track, as git difftool hands them over, and finds those that lie in the
// repository's worktree. It runs no git and does not decide what a
// comparison shows; the use case does.
package files
