package files

import (
	"path/filepath"

	"go.uber.org/fx"

	"github.com/dturcinovic/seam/internal/app/port/out"
)

// Config is the repository whose worktree LocateInWorktree finds files in.
type Config struct {
	// Root is the worktree's root; empty outside a repository.
	Root string
}

// Module provides the disk as the ports it fulfils.
var Module = fx.Module("files",
	fx.Provide(fx.Annotate(New,
		fx.As(new(out.PathLister)),
		fx.As(new(out.PathReader)),
		fx.As(new(out.WorktreeLocator)),
	)),
)

// Disk implements out.PathLister, out.PathReader and out.WorktreeLocator.
type Disk struct {
	root string
}

// New returns the disk for the repository of cfg. A root that cannot be
// resolved locates nothing, as if there were no repository.
func New(cfg Config) *Disk {
	if cfg.Root == "" {
		return &Disk{}
	}
	// Resolved, as the paths it is compared with are: the root itself may
	// lie below a link.
	root, err := filepath.EvalSymlinks(cfg.Root)
	if err != nil {
		return &Disk{}
	}
	return &Disk{root: root}
}
