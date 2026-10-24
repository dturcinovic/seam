package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/config"
	"seam/internal/domain/limits"
)

func TestLoad(t *testing.T) {
	home := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
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
			assert.Equal(t, wd, cfg.Dir)
		})
	}
}

func TestConfigFile(t *testing.T) {
	home := t.TempDir()
	tests := map[string]struct{ xdgConfig, want string }{
		"XDG_CONFIG_HOME set":                 {"/etc/xdg-user", "/etc/xdg-user/seam/config.toml"},
		"XDG_CONFIG_HOME unset":               {"", filepath.Join(home, ".config/seam/config.toml")},
		"relative XDG_CONFIG_HOME is ignored": {"conf", filepath.Join(home, ".config/seam/config.toml")},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", tt.xdgConfig)

			cfg, err := config.Load(config.Flags{})
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.ConfigFile)
		})
	}
}

func TestLoadWithoutHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	_, err := config.Load(config.Flags{})
	assert.ErrorContains(t, err, "XDG_STATE_HOME")
}

func TestFileSizeLimit(t *testing.T) {
	assert.Equal(t, limits.DefaultFileSize, config.Config{}.FileSizeLimit(), "not set")
	assert.Equal(t, limits.FileSize(9), config.Config{MaxFileSize: 9}.FileSizeLimit())
}

func TestMaxFileSize(t *testing.T) {
	tests := []struct {
		flag    string
		want    limits.FileSize
		wantErr bool
	}{
		{"", limits.DefaultFileSize, false},
		{"1000", 1000, false},
		{"16K", 16 << 10, false},
		{"16M", 16 << 20, false},
		{"2G", 2 << 30, false},
		{"0", 0, true},
		{"-1M", 0, true},
		{"16MB", 0, true},
		{"M", 0, true},
		{"x", 0, true},
		{"9999999999G", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())

			cfg, err := config.Load(config.Flags{MaxFileSize: tt.flag})

			if tt.wantErr {
				require.ErrorIs(t, err, limits.ErrInvalidFileSize)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.MaxFileSize)
		})
	}
}
