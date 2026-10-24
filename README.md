# seam

A side-by-side diff viewer for the terminal. One key switches between the
changed hunks and the whole file, and hovering over a symbol shows its type
and documentation from the language server.

Work in progress: the skeleton is in place, the diff is not yet.

## Build

Requires Go 1.27 and [Task](https://taskfile.dev).

```bash
task check    # the full gate
task build    # bin/seam
```

The log is written to `$XDG_STATE_HOME/seam/seam.log`.
