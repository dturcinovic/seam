// Package notestore keeps the book of review notes in one JSON file in
// git's directory, shared by every seam process of the worktree. It
// changes the book under a file lock and never writes over a file it
// cannot read. It knows no git: the composition root names the file.
package notestore
