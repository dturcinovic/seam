package config

import (
	"fmt"
	"os"
	"path/filepath"

	"seam/internal/domain/limits"
)

// Flags are the command-line switches that affect configuration.
type Flags struct {
	Verbose bool
	// MaxFileSize is a size in bytes, or with a binary suffix K, M or G;
	// empty means the default.
	MaxFileSize string
}

// Config is the resolved configuration of one session.
type Config struct {
	// Dir is the working directory; the repository is found from it.
	Dir string
	// LogFile is the zap log, rotated in place.
	LogFile string
	// ConfigFile is the TOML file of languages and servers; it need not
	// exist.
	ConfigFile string
	// Verbose raises the log level to debug. It does not mirror the log on
	// stderr: the TUI owns the terminal.
	Verbose bool
	// MaxFileSize is the largest side of a file that is loaded; zero means
	// the default. Read it through FileSizeLimit.
	MaxFileSize limits.FileSize
}

// FileSizeLimit is MaxFileSize, or the default if it is not set.
func (c Config) FileSizeLimit() limits.FileSize {
	if c.MaxFileSize <= 0 {
		return limits.DefaultFileSize
	}
	return c.MaxFileSize
}

// Load resolves the configuration. The log is state and lives under the XDG
// state directory, the configuration file under the XDG config directory.
func Load(f Flags) (Config, error) {
	state, err := xdgDir("XDG_STATE_HOME", ".local/state")
	if err != nil {
		return Config{}, err
	}
	conf, err := xdgDir("XDG_CONFIG_HOME", ".config")
	if err != nil {
		return Config{}, err
	}
	dir, err := os.Getwd()
	if err != nil {
		return Config{}, fmt.Errorf("working directory: %w", err)
	}
	size, err := parseSize(f.MaxFileSize)
	if err != nil {
		return Config{}, fmt.Errorf("--max-file-size: %w", err)
	}
	return Config{
		Dir:         dir,
		LogFile:     filepath.Join(state, "seam", "seam.log"),
		ConfigFile:  filepath.Join(conf, "seam", "config.toml"),
		Verbose:     f.Verbose,
		MaxFileSize: size,
	}, nil
}

// parseSize reads --max-file-size; empty is the default limit.
func parseSize(s string) (limits.FileSize, error) {
	if s == "" {
		return limits.DefaultFileSize, nil
	}
	return limits.ParseFileSize(s)
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
