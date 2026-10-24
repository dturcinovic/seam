package observability

import "go.uber.org/fx"

// Module hands the supplied logger to the graph. The logger is built before
// the graph and closed after it: fx logs its own events up to the very end
// of Stop, past any hook that could close it.
var Module = fx.Module("observability",
	fx.Provide(
		(*Logger).Zap,
		PanicFunc,
	),
)
