package lsp

import (
	"encoding/json/jsontext"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
	"seam/internal/testsupport"
)

func TestTheOldestWorkUnderWayIsReported(t *testing.T) {
	indexing := hover.Progress{Title: "indexing", Percent: 40, Known: true}
	tests := []struct {
		name    string
		params  []string
		want    hover.Progress
		changed bool
	}{
		{"begin", []string{`{"token":"a","value":{"kind":"begin","title":"indexing","percentage":40}}`}, indexing, true},
		{"report", []string{
			`{"token":"a","value":{"kind":"begin","title":"indexing"}}`,
			`{"token":"a","value":{"kind":"report","percentage":40}}`,
		}, indexing, true},
		{"end", []string{
			`{"token":"a","value":{"kind":"begin","title":"indexing"}}`,
			`{"token":"a","value":{"kind":"end"}}`,
		}, hover.Progress{}, true},
		{"later work waits behind the oldest", []string{
			`{"token":1,"value":{"kind":"begin","title":"indexing","percentage":40}}`,
			`{"token":2,"value":{"kind":"begin","title":"cargo check"}}`,
			`{"token":2,"value":{"kind":"report","percentage":90}}`,
		}, indexing, false},
		{"the next shows once the oldest ends", []string{
			`{"token":1,"value":{"kind":"begin","title":"indexing"}}`,
			`{"token":2,"value":{"kind":"begin","title":"cargo check"}}`,
			`{"token":1,"value":{"kind":"end"}}`,
		}, hover.Progress{Title: "cargo check"}, true},
		{"no title", []string{`{"token":"a","value":{"kind":"begin"}}`}, hover.Progress{Title: untitled}, true},
		{"a fraction", []string{`{"token":"a","value":{"kind":"begin","title":"x","percentage":12.5}}`}, hover.Progress{Title: "x", Percent: 13, Known: true}, true},
		{"a percentage past 100", []string{`{"token":"a","value":{"kind":"begin","title":"x","percentage":250}}`}, hover.Progress{Title: "x", Percent: 100, Known: true}, true},
		{"a report of no work", []string{`{"token":"b","value":{"kind":"report","percentage":5}}`}, hover.Progress{}, false},
		{"partial results", []string{`{"token":"b","value":[1,2]}`}, hover.Progress{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w works
			var got hover.Progress
			var changed bool
			for _, p := range tt.params {
				got, changed = w.take(jsontext.Value(p))
			}
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.changed, changed, "changed by the last")
		})
	}
}

func lastProgress(all []hover.Update) (hover.Progress, bool) {
	var p hover.Progress
	seen := false
	for _, u := range all {
		if u.Kind == hover.ProgressChanged {
			p, seen = u.Progress, true
		}
	}
	return p, seen
}

func TestAServersProgressIsReportedUntilItEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		step := make(chan string)
		pool := testPool(t, goTable(t, lang.Server{}), &pipes{serve: func(p *peer) {
			p.serveUntil(func(m message) bool {
				if m.Method == "initialized" {
					for params := range step {
						p.send(message{Method: "$/progress", Params: jsontext.Value(params)})
					}
				}
				return true
			})
		}})
		inst, ok := pool.Sync(doc("a.go", 1, "package a\n"))
		require.True(t, ok)
		testsupport.Collect(t, pool, hasStatus(hover.Ready))

		step <- `{"token":"t","value":{"kind":"begin","title":"indexing","percentage":0}}`
		step <- `{"token":"t","value":{"kind":"report","percentage":40}}`
		synctest.Wait()
		all := pool.Drain()
		got, _ := lastProgress(all)
		assert.Equal(t, hover.Progress{Title: "indexing", Percent: 40, Known: true}, got)
		require.Equal(t, 1, countKind(all, hover.ProgressChanged), "a report not drained yet is replaced")
		assert.Equal(t, inst, all[len(all)-1].Instance)

		step <- `{"token":"t","value":{"kind":"end"}}`
		synctest.Wait()
		got, seen := lastProgress(pool.Drain())
		assert.True(t, seen)
		assert.Zero(t, got, "no work is left")
		close(step)
		stopPool(t, pool)
	})
}

func countKind(all []hover.Update, k hover.UpdateKind) int {
	n := 0
	for _, u := range all {
		if u.Kind == k {
			n++
		}
	}
	return n
}

func TestPendingProgressKeepsItsEndsAndOrder(t *testing.T) {
	gopls := lang.Instance{Server: "gopls"}
	work := func(title string) hover.Progress { return hover.Progress{Title: title} }
	tests := []struct {
		name string
		add  func(e *events)
		want []hover.Update
	}{
		{"a report replaces the one just before", func(e *events) {
			e.progressLocked(gopls, work("a"))
			e.progressLocked(gopls, work("b"))
		}, []hover.Update{{Kind: hover.ProgressChanged, Instance: gopls, Progress: work("b")}}},
		{"the end of work stays", func(e *events) {
			e.progressLocked(gopls, work("a"))
			e.progressLocked(gopls, hover.Progress{})
			e.progressLocked(gopls, work("b"))
		}, []hover.Update{
			{Kind: hover.ProgressChanged, Instance: gopls},
			{Kind: hover.ProgressChanged, Instance: gopls, Progress: work("b")},
		}},
		{"nothing moves past an answer", func(e *events) {
			e.progressLocked(gopls, work("a"))
			e.pending = append(e.pending, hover.Update{Kind: hover.HoverDone, Req: 1})
			e.progressLocked(gopls, work("b"))
		}, []hover.Update{
			{Kind: hover.ProgressChanged, Instance: gopls, Progress: work("a")},
			{Kind: hover.HoverDone, Req: 1},
			{Kind: hover.ProgressChanged, Instance: gopls, Progress: work("b")},
		}},
		{"a stop ends the work, even when a restart follows", func(e *events) {
			e.progressLocked(gopls, work("a"))
			e.statusLocked(gopls, hover.Stopped)
			e.statusLocked(gopls, hover.Starting)
			e.statusLocked(gopls, hover.Ready)
		}, []hover.Update{
			{Kind: hover.ProgressChanged, Instance: gopls, Progress: work("a")},
			{Kind: hover.ProgressChanged, Instance: gopls},
			{Kind: hover.StatusChanged, Instance: gopls, Status: hover.Ready},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEvents()
			tt.add(e)
			assert.Equal(t, tt.want, e.Drain())
		})
	}
}

func TestOnlyAReadyClientReportsWork(t *testing.T) {
	for _, st := range []hover.ServerStatus{hover.Starting, hover.Stopped, hover.ServerFailed} {
		e := newEvents()
		c := &client{inst: lang.Instance{Server: "gopls"}, state: st}
		e.of(c).progress(c.inst, hover.Progress{Title: "indexing"})
		assert.Empty(t, e.Drain(), st)
	}
	e := newEvents()
	c := &client{inst: lang.Instance{Server: "gopls"}, state: hover.Ready}
	e.of(c).progress(c.inst, hover.Progress{Title: "indexing"})
	assert.Len(t, e.Drain(), 1)
}
