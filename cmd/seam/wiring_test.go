package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/dturcinovic/seam/internal/adapter/driven/configfile"
	"github.com/dturcinovic/seam/internal/app/port/in"
	"github.com/dturcinovic/seam/internal/app/usecase"
	"github.com/dturcinovic/seam/internal/config"
	"github.com/dturcinovic/seam/internal/domain/lang"
)

// TestTheGraphProvidesEveryIncomingPort checks the wiring without running
// it: a port nothing asks for yet would otherwise fail only when the TUI
// first does.
func TestTheGraphProvidesEveryIncomingPort(t *testing.T) {
	var (
		loader   in.ChangesetLoader
		reloader in.ChangesetReloader
		files    in.FileDiffLoader
		hoverer  in.Hoverer
		docs     in.DocumentTracker
		updates  in.LSPUpdates
	)
	dir := t.TempDir()
	opts := append(options(config.Config{}, nil, dir, usecase.NotesSettings{}, configfile.File{Languages: lang.Defaults()}),
		fx.Populate(&loader, &reloader, &files, &hoverer, &docs, &updates))

	require.NoError(t, fx.ValidateApp(opts...))
}
