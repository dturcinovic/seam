package lsp

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/dturcinovic/seam/internal/domain/hover"
)

// location is the part of a Location or a LocationLink seam reads: where
// the definition starts. A link names its target's selection, the name
// itself, rather than the whole definition.
type location struct {
	URI   string `json:"uri"`
	Range struct {
		Start struct {
			Line int `json:"line"`
		} `json:"start"`
	} `json:"range"`
	TargetURI            string `json:"targetUri"`
	TargetSelectionRange struct {
		Start struct {
			Line int `json:"line"`
		} `json:"start"`
	} `json:"targetSelectionRange"`
}

// definitionResult answers p with the first of the locations the server
// sent that names a file: null, a Location, or a list of Locations or
// LocationLinks.
func (c *client) definitionResult(p *pending, m message) hover.Update {
	u := p.ended(hover.Error)
	if m.Error != nil {
		return u
	}
	locs, err := locations(m.Result)
	if err != nil {
		c.log.Info("definition result: " + err.Error())
		return u
	}
	if len(locs) == 0 {
		u.Outcome = hover.Empty
		return u
	}
	for _, l := range locs {
		if target, ok := c.located(l); ok {
			u.Outcome, u.Target, u.Targets = hover.Text, target, len(locs)
			return u
		}
	}
	return u
}

func locations(result jsontext.Value) ([]location, error) {
	// An if chain rather than a switch: the other kinds are no answer, and
	// unmarshalling them into the list fails as it should.
	if len(result) == 0 || result.Kind() == 'n' {
		return nil, nil
	}
	if result.Kind() == '{' {
		var l location
		err := json.Unmarshal(result, &l)
		return []location{l}, err
	}
	var ls []location
	err := json.Unmarshal(result, &ls)
	return ls, err
}

// located is l as a place in the tree the client's server runs in, or
// outside it; ok is false for a URI that names no file.
func (c *client) located(l location) (hover.Location, bool) {
	raw, line := l.URI, l.Range.Start.Line
	if l.TargetURI != "" {
		raw, line = l.TargetURI, l.TargetSelectionRange.Start.Line
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "file" {
		return hover.Location{}, false
	}
	abs := filepath.FromSlash(u.Path)
	rel, err := filepath.Rel(c.repoRoot, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return hover.Location{Path: abs, Outside: true, Line: line}, true
	}
	return hover.Location{Commit: c.inst.Commit, Path: filepath.ToSlash(rel), Line: line}, true
}
