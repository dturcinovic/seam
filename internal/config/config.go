package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Flags are the command-line switches that affect configuration.
type Flags struct {
	Verbose bool
}

// Config is the resolved configuration of one session.
type Config struct {
	// LogFile is the zap log, rotated in place.
	LogFile string
	// Verbose raises the log level to debug. It does not mirror the log on
	// stderr: the TUI owns the terminal.
	Verbose bool
}

// Load resolves the configuration. The log is state and lives under the XDG
// state directory.
func Load(f Flags) (Config, error) {
	state, err := xdgDir("XDG_STATE_HOME", ".local/state")
	if err != nil {
		return Config{}, err
	}
	return Config{
		LogFile: filepath.Join(state, "seam", "seam.log"),
		Verbose: f.Verbose,
	}, nil
}

func xdgDir(env, fallback string) (string, error) {
	// The XDG spec says a relative value is invalid and must be ignored.
	if dir := os.Getenv(env); filepath.IsAbs(dir) {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory for %s: %w", env, err)
	}
	return filepath.Join(home, filepath.FromSlash(fallback)), nil
}
