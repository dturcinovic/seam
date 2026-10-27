package notestore

import (
	"go.uber.org/fx"

	"github.com/dturcinovic/seam/internal/app/port/out"
)

// Config names the file of the book; empty outside a repository, where
// there are no notes.
type Config struct {
	File string
}

// Module provides the store as out.NoteStore.
var Module = fx.Module("notestore",
	fx.Provide(fx.Annotate(New, fx.As(new(out.NoteStore)))),
)
