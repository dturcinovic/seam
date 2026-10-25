# Language servers

Hover and go-to-definition need a language server for the file's language.
seam installs none and starts one only when a file of its language is
shown. A server that is missing or turned off costs the hover and nothing
else: the diff, navigation and syntax colours work without it, and the
footer says which server a file has.

| Files | Server (name in the configuration) | Command | Install |
|---|---|---|---|
| `.go` | `gopls` | `gopls` | `go install golang.org/x/tools/gopls@latest` |
| `.rs` | `rust-analyzer` | `rust-analyzer` | `rustup component add rust-analyzer` |
| `.js` `.mjs` `.cjs` `.ts` `.mts` `.cts` `.jsx` `.tsx` | `tsgo` | `tsc --lsp --stdio` | `npm install -g typescript@7` |
| `.html` `.htm` | `vscode-html-language-server` | `vscode-html-language-server --stdio` | `npm install -g vscode-langservers-extracted` |
| `.php` | `intelephense` | `intelephense --stdio` | `npm install -g intelephense` |
| `.py` `.pyi` | `pyright` | `pyright-langserver --stdio` | `npm install -g pyright` |

A file without one of these extensions is recognised by its first line too:
a shebang naming `node`, `php` or `python`, or `<?php`.

The npm servers need Node.js on the `PATH`. tsgo is TypeScript 7's native
compiler; a `tsc` older than TypeScript 7 on the `PATH` has no `--lsp` and
fails to start. To use another server, or one that is not on the `PATH`,
see [Configuration](configuration.md#language-servers-and-languages).

## Workspaces

Each server runs once per workspace: the nearest directory above the file
that holds one of its root markers, else the repository root.

| Server | Root markers |
|---|---|
| `gopls` | `go.work`, `go.mod` |
| `rust-analyzer` | `Cargo.toml` |
| `tsgo` | `tsconfig.json`, `jsconfig.json`, `package.json` |
| `intelephense` | `composer.json` |
| `pyright` | `pyproject.toml` |
| `vscode-html-language-server` | none: the repository root |

JavaScript, TypeScript, JSX and TSX of one workspace share one server. At
most four servers run at once (`max_instances` under `[limits]`); the one
used longest ago makes room for a new one, and starts again when it is
needed. A server none of whose files seam still holds open ends after a
minute.

## Comparing commits

When a side is a commit rather than the worktree, the servers read a
checkout of it that seam makes under `$XDG_CACHE_HOME/seam/worktrees/` and
removes when it ends. A checkout holds only tracked files: without
`node_modules` or `vendor/` the servers answer about the project's own
code, not about its packages.

## Python

pyright finds the packages of a project only in the Python environment it
is told about:

```toml
[servers.pyright.settings.python]
pythonPath = ".venv/bin/python"
```
