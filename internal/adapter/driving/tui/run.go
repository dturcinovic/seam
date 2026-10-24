package tui

import (
	"context"
	"errors"
	"io"

	tea "charm.land/bubbletea/v2"

	"seam/internal/app/port/in"
	"seam/internal/domain/change"
	"seam/internal/supervise"
)

// Ports are the incoming ports the TUI calls while it runs.
type Ports struct {
	Files  in.FileDiffLoader
	Reload in.ChangesetReloader
	Docs   in.DocumentTracker
	LSP    in.LSPUpdates
	Hover  in.Hoverer
}

// IO is where the program reads keys and writes the screen; nil means the
// terminal. Tests set pipes and a size, as a pipe reports none.
type IO struct {
	In            io.Reader
	Out           io.Writer
	Width, Height int
}

// Run shows cs until the user quits or ctx ends, which is no error. It owns
// the program and the bridge that wakes it, and returns with both ended and
// the terminal restored; the use cases' workers are the caller's to stop.
// Panics in the bridge go to panics.
func Run(ctx context.Context, cs change.Changeset, p Ports, panics supervise.PanicFunc, term IO) error {
	// Signals end the program through ctx alone: Bubble Tea's own handler
	// would race main's and turn the same signal into an error or not.
	opts := []tea.ProgramOption{tea.WithContext(ctx), tea.WithoutSignalHandler()}
	if term.In != nil {
		opts = append(opts, tea.WithInput(term.In))
	}
	if term.Out != nil {
		opts = append(opts, tea.WithOutput(term.Out))
	}
	if term.Width > 0 {
		opts = append(opts, tea.WithWindowSize(term.Width, term.Height))
	}
	prog := tea.NewProgram(newModel(cs, p), opts...)
	b := newBridge(panics)
	if err := b.start(prog.Send, p); err != nil {
		// The program never ran, so nothing holds up the watcher started.
		return errors.Join(err, b.stop())
	}
	_, err := prog.Run()
	if ctx.Err() != nil && errors.Is(err, tea.ErrProgramKilled) {
		err = nil
	}
	return errors.Join(err, b.stop())
}
