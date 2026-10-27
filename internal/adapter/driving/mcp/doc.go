// Package mcp serves the review notes of a worktree to an agent session
// (Claude Code, OpenCode, any MCP client) over the Model Context
// Protocol on stdin and stdout: the agent takes notes, reads them and
// answers them. It calls the incoming ports only and keeps no notes of its
// own. It runs no TUI and no language server, and writes nothing to stdout
// but the protocol.
package mcp
