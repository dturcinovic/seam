package config_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/config"
)

func TestLoad(t *testing.T) {
	home := t.TempDir()
	tests := []struct {
		name     string
		xdgState string
		want     string
	}{
		{name: "XDG_STATE_HOME set", xdgState: "/var/state", want: "/var/state/seam"},
		{name: "XDG_STATE_HOME unset", xdgState: "", want: filepath.Join(home, ".local/state/seam")},
		{name: "relative XDG_STATE_HOME is ignored", xdgState: "state", want: filepath.Join(home, ".local/state/seam")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", home)
			t.Setenv("XDG_STATE_HOME", tt.xdgState)

			cfg, err := config.Load(config.Flags{Verbose: true})
			require.NoError(t, err)

			assert.Equal(t, filepath.Join(tt.want, "seam.log"), cfg.LogFile)
			assert.True(t, cfg.Verbose)
		})
	}
}

func TestLoadWithoutHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	_, err := config.Load(config.Flags{})
	assert.ErrorContains(t, err, "XDG_STATE_HOME")
}
