package chroma

import (
	"go.uber.org/fx"

	"github.com/dturcinovic/seam/internal/app/port/out"
)

// Module provides the highlighter as the port it fulfils.
var Module = fx.Module("chroma",
	fx.Provide(fx.Annotate(New, fx.As(new(out.Highlighter)))),
)
