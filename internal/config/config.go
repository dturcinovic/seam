package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dturcinovic/seam/internal/domain/limits"
)

// Flags are the command-line switches that affect configuration.
type Flags struct {
	Verbose bool
	// MaxFileSize is a size in bytes, or with a binary suffix K, M or G;
	// empty means the default.
	MaxFileSize string
	// HeapProfile is where to write a heap profile at the end of the session;
	// empty means none.
	HeapProfile string
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
	// Checkouts holds the checkouts of commits that hovers ask in.
	Checkouts string
	// Verbose raises the log level to debug. It does not mirror the log on
	// stderr: the TUI owns the terminal.
	Verbose bool
	// MaxFileSize is the largest side of a file that is loaded; zero means
	// the default. Read it through FileSizeLimit.
	MaxFileSize limits.FileSize
	// HeapProfile is the file the session writes its heap profile to as
	// the TUI ends; empty means none.
	HeapProfile string
	// ReleaseFile keeps the last check for a newer release.
	ReleaseFile string
	// UpdateCheck is off when SEAM_NO_UPDATE_CHECK is set, for machines
	// without a configuration file of their own, such as CI, or when the
	// file turns it off (Settle).
	UpdateCheck bool
	// UpdateURL is SEAM_UPDATE_URL, which replaces GitHub's API for the
	// check, as for a mirror or a test server; empty means GitHub.
	UpdateURL string
}

// Settle takes what the configuration file sets: its file size limit, zero
// if it sets none, unless the flag set one, as a flag is the nearer intent;
// and whether to check for updates, which either of file and environment
// can turn off.
func (c Config) Settle(maxFileSize limits.FileSize, updateCheck bool) Config {
	if c.MaxFileSize == 0 {
		c.MaxFileSize = maxFileSize
	}
	c.UpdateCheck = c.UpdateCheck && updateCheck
	return c
}

// FileSizeLimit is MaxFileSize, or the default if it is not set.
func (c Config) FileSizeLimit() limits.FileSize {
	if c.MaxFileSize <= 0 {
		return limits.DefaultFileSize
	}
	return c.MaxFileSize
}

// Load resolves the configuration. The log is state and lives under the XDG
// state directory, the configuration file under the XDG config directory,
// the checkouts, which can always be made again, under the cache directory.
func Load(f Flags) (Config, error) {
	state, err := xdgDir("XDG_STATE_HOME", ".local/state")
	if err != nil {
		return Config{}, err
	}
	conf, err := xdgDir("XDG_CONFIG_HOME", ".config")
	if err != nil {
		return Config{}, err
	}
	cache, err := xdgDir("XDG_CACHE_HOME", ".cache")
	if err != nil {
		return Config{}, err
	}
	dir, err := os.Getwd()
	if err != nil {
		return Config{}, fmt.Errorf("working directory: %w", err)
	}
	var size limits.FileSize
	if f.MaxFileSize != "" {
		if size, err = limits.ParseFileSize(f.MaxFileSize); err != nil {
			return Config{}, fmt.Errorf("--max-file-size: %w", err)
		}
	}
	return Config{
		Dir:         dir,
		LogFile:     filepath.Join(state, "seam", "seam.log"),
		ConfigFile:  filepath.Join(conf, "seam", "config.toml"),
		Checkouts:   filepath.Join(cache, "seam", "worktrees"),
		Verbose:     f.Verbose,
		MaxFileSize: size,
		HeapProfile: f.HeapProfile,
		ReleaseFile: filepath.Join(state, "seam", "release.json"),
		UpdateCheck: !turnedOff(os.Getenv("SEAM_NO_UPDATE_CHECK")),
		UpdateURL:   os.Getenv("SEAM_UPDATE_URL"),
	}, nil
}

// turnedOff reads a SEAM_NO_* switch: set means off, but 0 and false, as
// a template might write to mean "not set", leave it on.
func turnedOff(value string) bool {
	switch strings.ToLower(value) {
	case "", "0", "false":
		return false
	}
	return true
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
