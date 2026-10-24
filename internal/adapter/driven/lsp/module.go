package lsp

import (
	"go.uber.org/fx"
	"go.uber.org/zap"

	"seam/internal/app/port/out"
	"seam/internal/domain/lang"
	"seam/internal/supervise"
)

// Module provides the pool as the ports it fulfils and stops its servers
// with the graph.
var Module = fx.Module("lsp",
	fx.Provide(fx.Annotate(newModulePool,
		fx.As(new(out.DocumentSync)),
		fx.As(new(out.HoverRequester)),
		fx.As(new(out.ServerEvents)),
	)),
)

func newModulePool(lc fx.Lifecycle, cfg Config, table lang.Table, log *zap.Logger, panics supervise.PanicFunc) *Pool {
	p := newPool(cfg, table, newProcesses(cfg.LogDir), log, panics)
	lc.Append(fx.StopHook(p.Stop))
	return p
}
