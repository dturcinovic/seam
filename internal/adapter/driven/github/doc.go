// Package github asks GitHub's public release API for the latest release of
// seam. It sends nothing about the user but seam's version in the
// User-Agent, which GitHub requires, and it does not decide when to ask.
package github
