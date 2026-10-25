package lsp

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"math"
	"slices"

	"seam/internal/domain/hover"
)

// untitled stands in for the title of work a server begins without one:
// work under way must not look like none.
const untitled = "working"

// work is one piece of work a server reports, by its token.
type work struct {
	token    string
	progress hover.Progress
}

// progressParams is what seam reads of $/progress: work done progress.
// Other values, as partial results, have no kind seam knows.
type progressParams struct {
	Token jsontext.Value `json:"token"`
	Value struct {
		Kind       string   `json:"kind"`
		Title      string   `json:"title"`
		Percentage *float64 `json:"percentage"`
	} `json:"value"`
}

// works is the work a server has under way, oldest first. Only the reader
// goroutine touches it, so it needs no lock.
type works struct {
	list     []work
	reported hover.Progress
}

// take applies $/progress and says what to report: the oldest work under
// way, the zero value for none; changed is false when that stays the same,
// as most reports of a percentage of later work do.
func (w *works) take(params jsontext.Value) (shown hover.Progress, changed bool) {
	var p progressParams
	if err := json.Unmarshal(params, &p); err != nil {
		// A server's malformed progress costs it only the footer's word.
		return w.reported, false
	}
	token := string(p.Token)
	at := slices.IndexFunc(w.list, func(x work) bool { return x.token == token })
	switch v := p.Value; {
	case v.Kind == "begin" && at < 0:
		title := v.Title
		if title == "" {
			title = untitled
		}
		w.list = append(w.list, work{token: token, progress: percent(hover.Progress{Title: title}, v.Percentage)})
	case v.Kind == "report" && at >= 0:
		w.list[at].progress = percent(w.list[at].progress, v.Percentage)
	case v.Kind == "end" && at >= 0:
		w.list = slices.Delete(w.list, at, at+1)
	}
	shown = hover.Progress{}
	if len(w.list) > 0 {
		shown = w.list[0].progress
	}
	changed, w.reported = shown != w.reported, shown
	return shown, changed
}

// percent sets the percentage of p, if pct gives one. The spec has a
// whole number from 0 to 100; a server sending a fraction still counts.
func percent(p hover.Progress, pct *float64) hover.Progress {
	if pct != nil {
		p.Percent, p.Known = int(min(max(math.Round(*pct), 0), 100)), true
	}
	return p
}
