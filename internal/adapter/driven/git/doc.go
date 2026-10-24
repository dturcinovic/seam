// Package git reads the repository seam compares in: it lists changes and
// untracked files with the git command and reads blobs through one
// long-running cat-file process and worktree files through os.Root. It never
// writes to the repository and never takes its index lock.
package git
