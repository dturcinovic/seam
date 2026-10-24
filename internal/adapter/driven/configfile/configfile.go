package configfile

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"seam/internal/domain/lang"
)

// document is the file as written. A pointer or map left nil is a key the
// file does not set, which keeps the default.
type document struct {
	Servers   map[string]serverEntry   `toml:"servers"`
	Languages map[string]languageEntry `toml:"languages"`
}

type serverEntry struct {
	Command               *string        `toml:"command"`
	Args                  *[]string      `toml:"args"`
	Enabled               *bool          `toml:"enabled"`
	RootMarkers           *[]string      `toml:"root_markers"`
	InitializationOptions map[string]any `toml:"initialization_options"`
	Settings              map[string]any `toml:"settings"`
}

type languageEntry struct {
	Extensions *[]string `toml:"extensions"`
	LanguageID *string   `toml:"language_id"`
	Server     *string   `toml:"server"`
}

// Load reads the configuration file at path over the default table of
// languages and servers. A missing file is the defaults; a link to one is
// an error, as it is a configuration someone meant to have.
func Load(path string) (lang.Table, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		if _, lerr := os.Lstat(path); lerr == nil {
			return lang.Table{}, fmt.Errorf("read configuration: %s links to a missing file", path)
		}
		return lang.Defaults(), nil
	}
	if err != nil {
		return lang.Table{}, fmt.Errorf("read configuration: %w", err)
	}
	var d document
	// Unknown keys are refused: a misspelt one would otherwise change
	// nothing, without a word.
	if err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&d); err != nil {
		return lang.Table{}, decodeError(path, err)
	}
	t, err := d.table()
	if err != nil {
		return lang.Table{}, fmt.Errorf("%s: %w", path, err)
	}
	return t, nil
}

// decodeError names the place in the file, for each unknown key too.
func decodeError(path string, err error) error {
	if missing, ok := errors.AsType[*toml.StrictMissingError](err); ok {
		all := make([]error, 0, len(missing.Errors))
		for _, e := range missing.Errors {
			row, col := e.Position()
			all = append(all, fmt.Errorf("%s:%d:%d: unknown key %s", path, row, col, strings.Join(e.Key(), ".")))
		}
		return errors.Join(all...)
	}
	if de, ok := errors.AsType[*toml.DecodeError](err); ok {
		row, col := de.Position()
		if key := de.Key(); len(key) > 0 {
			return fmt.Errorf("%s:%d:%d: %s: %w", path, row, col, strings.Join(key, "."), err)
		}
		return fmt.Errorf("%s:%d:%d: %w", path, row, col, err)
	}
	return fmt.Errorf("%s: %w", path, err)
}

// table lays the document over the defaults, entry by entry in the order
// of their names, so an error names the same entry on every run.
func (d document) table() (lang.Table, error) {
	defaults := lang.Defaults()
	servers := defaults.Servers()
	for _, name := range slices.Sorted(maps.Keys(d.Servers)) {
		i := slices.IndexFunc(servers, func(s lang.Server) bool { return s.Name == name })
		if i < 0 {
			servers = append(servers, lang.Server{Name: name})
			i = len(servers) - 1
		}
		d.Servers[name].apply(&servers[i])
	}
	languages := defaults.Languages()
	for _, name := range slices.Sorted(maps.Keys(d.Languages)) {
		i := slices.IndexFunc(languages, func(l lang.Language) bool { return string(l.Lang) == name })
		if i < 0 {
			languages = append(languages, lang.Language{Lang: lang.Lang(name)})
			i = len(languages) - 1
		}
		d.Languages[name].apply(&languages[i])
	}
	return lang.NewTable(languages, servers)
}

func (e serverEntry) apply(s *lang.Server) {
	set(&s.Command, e.Command)
	set(&s.Args, e.Args)
	set(&s.RootMarkers, e.RootMarkers)
	if e.Enabled != nil {
		s.Disabled = !*e.Enabled
	}
	if e.InitializationOptions != nil {
		s.InitializationOptions = e.InitializationOptions
	}
	if e.Settings != nil {
		s.Settings = e.Settings
	}
}

func (e languageEntry) apply(l *lang.Language) {
	set(&l.Extensions, e.Extensions)
	set(&l.LanguageID, e.LanguageID)
	set(&l.Server, e.Server)
}

func set[T any](dst *T, v *T) {
	if v != nil {
		*dst = *v
	}
}
