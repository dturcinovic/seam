package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"seam/internal/app/port/in"
	"seam/internal/config"
)

// TestTheGraphProvidesEveryIncomingPort checks the wiring without running
// it: a port nothing asks for yet would otherwise fail only when the TUI
// first does.
func TestTheGraphProvidesEveryIncomingPort(t *testing.T) {
	var (
		loader   in.ChangesetLoader
		reloader in.ChangesetReloader
		files    in.FileDiffLoader
	)
	opts := append(options(config.Config{}, nil, t.TempDir()), fx.Populate(&loader, &reloader, &files))

	require.NoError(t, fx.ValidateApp(opts...))
}
