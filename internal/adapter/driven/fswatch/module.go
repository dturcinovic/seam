package fswatch

import (
	"go.uber.org/fx"

	"github.com/dturcinovic/seam/internal/app/port/out"
)

// Module provides the watchers as out.ChangeWatcher and out.NoteWatcher.
// They hold nothing until they watch, and watching ends with its context.
var Module = fx.Module("fswatch",
	fx.Provide(fx.Annotate(New, fx.As(new(out.ChangeWatcher)))),
	fx.Provide(fx.Annotate(NewNoteWatcher, fx.As(new(out.NoteWatcher)))),
)
