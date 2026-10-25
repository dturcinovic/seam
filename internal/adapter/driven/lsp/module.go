package lsp

import (
	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/dturcinovic/seam/internal/app/port/out"
	"github.com/dturcinovic/seam/internal/domain/lang"
	"github.com/dturcinovic/seam/internal/supervise"
)

// Module provides the pool as the ports it fulfils and stops its servers
// with the graph.
var Module = fx.Module("lsp",
	fx.Provide(fx.Annotate(newModulePool,
		fx.As(new(out.DocumentSync)),
		fx.As(new(out.HoverRequester)),
		fx.As(new(out.DefinitionRequester)),
		fx.As(new(out.ServerEvents)),
		fx.As(new(out.CheckoutRoots)),
	)),
)

func newModulePool(lc fx.Lifecycle, cfg Config, table lang.Table, log *zap.Logger, panics supervise.PanicFunc) *Pool {
	p := newPool(cfg, table, newProcesses(cfg.LogDir), log, panics)
	lc.Append(fx.StopHook(p.Stop))
	return p
}
