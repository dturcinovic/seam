package github

import (
	"go.uber.org/fx"

	"github.com/dturcinovic/seam/internal/app/port/out"
)

// Config is where to ask and what seam calls itself.
type Config struct {
	// BaseURL is GitHub's API, or a mirror or test server in its place;
	// empty means https://api.github.com.
	BaseURL string
	// Repo is owner/name of the repository whose releases count.
	Repo string
	// Agent is the User-Agent, such as seam/v0.1.0.
	Agent string
}

// Module provides the client as out.ReleaseFetcher.
var Module = fx.Module("github",
	fx.Provide(fx.Annotate(New, fx.As(new(out.ReleaseFetcher)))),
)
