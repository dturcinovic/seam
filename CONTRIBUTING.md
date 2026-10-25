# Contributing

Issues and pull requests are welcome. For a larger change, open an issue
first, so we can agree on the approach before you write the code.

## Setup

You need Go 1.27 and [Task](https://taskfile.dev). Everything else,
golangci-lint among it, runs through `go run` or `go tool` at the version
the repository pins; nothing is installed.

```bash
git config core.hooksPath .githooks   # the pre-commit hook runs task check
task build                            # bin/seam
```

## The gate

```bash
task check      # what a change must pass: generated code up to date, then
                # format, vet, lint, API snapshot, modules and licenses,
                # tests and the static build in parallel, coverage last
task test       # tests with the race detector, writes bin/cover.out
task lint       # golangci-lint with the layering rules
task generate   # regenerate the counterfeiter fakes
task api:bless  # accept a changed port or domain API
task fix        # apply the go fix modernizers
```

The pre-commit hook runs `task check` on the working tree, so stage or
stash everything else before you commit.

## Architecture

seam is built as ports and adapters:

```
internal/domain               the diff, views, anchors, languages, limits and
                              view settings; standard library only
internal/app/port/in          what the TUI calls
internal/app/port/out         what the use cases need from the outside
internal/app/usecase          fulfils port/in through port/out
internal/adapter/driven/*     fulfils port/out, one package per outside
                              system: git, chroma, lsp, configfile, fswatch
internal/adapter/driving/tui  Bubble Tea; calls only port/in
internal/supervise            goroutines and timers with an owner
internal/textcell             cell widths, control characters, overlays
cmd/seam/wiring.go            the one place that knows use cases and
                              adapters at once
```

depguard in `.golangci.yml` enforces the imports: Bubble Tea and Lip Gloss
only in `driving/tui`, chroma only in `driven/chroma`, fsnotify only in
`driven/fswatch`, and no adapter imports another.

## Rules the linters cannot tell you

- **Ports are narrow:** one to three methods, defined where they are used.
  A port grows by a new port, not by another method. Each port carries
  `//counterfeiter:generate . <Name>`; the fakes are committed.
- **API changes are visible:** the API of the ports and the domain is kept
  in `api/`. A change shows as a diff there and is accepted with
  `task api:bless`.
- **Goroutines have an owner:** every goroutine and timer callback starts
  through `supervise.Group`. A bare `go` statement fails `archtest`, and
  `time.AfterFunc` fails the linter.
- **The terminal belongs to the TUI:** outside `cmd/`, nothing writes to
  stdout or stderr; logging goes to the log file through zap.
- **Errors carry context:** wrap with `%w`; domain errors are named values
  or types checked with `errors.Is` and `errors.AsType`. An error that is
  ignored on purpose says why.
- **Comments say why, not what.** Every exported identifier has a doc
  comment, every package a `doc.go` saying what it does and what it does
  not.
- **No file over 500 lines**, tests included; no package variables except
  `Module` and sentinel errors; no `init()`.

## Tests

- testify, table tests, behaviour rather than implementation.
- Use cases against fakes, adapters against the real thing: git in a
  temporary repository, a fake language server (`fakelsp`).
- `testing/synctest` for anything with timers; goleak in `TestMain` of
  every package with goroutines.
- rapid for properties of the domain, golden files for rendering.
- `domain` and `usecase` keep at least 85 % coverage per package.

A change to what you see is tried in a terminal, not only in tests.
