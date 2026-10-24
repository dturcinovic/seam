// Package observability builds the process logger and routes fx's events and
// the panics caught by supervise into it. stdout and stderr belong to the
// TUI, so the log goes to a file only, also with --verbose; nothing here
// writes to the terminal.
package observability
