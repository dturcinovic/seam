package main

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDescribe(t *testing.T) {
	vcs := func(modified string) []debug.BuildSetting {
		return []debug.BuildSetting{
			{Key: "vcs.revision", Value: "1eac307a9c0e5b2d4f6a8c1e3b5d7f9a0c2e4b6d"},
			{Key: "vcs.time", Value: "2026-10-25T14:11:10Z"},
			{Key: "vcs.modified", Value: modified},
		}
	}
	tests := []struct {
		name    string
		stamped string
		bi      *debug.BuildInfo
		want    string
	}{
		{"release build of a tag", "v0.1.0", &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}, Settings: vcs("false")}, "v0.1.0 (1eac307, 2026-10-25)"},
		{"task build after the tag", "v0.1.0-3-g1eac307", &debug.BuildInfo{Settings: vcs("false")}, "v0.1.0-3-g1eac307 (1eac307, 2026-10-25)"},
		{"plain go build with changes", "", &debug.BuildInfo{Main: debug.Module{Version: "v0.1.1-0.20261025141110-1eac307a9c0e+dirty"}, Settings: vcs("true")}, "dev (1eac307-dirty, 2026-10-25)"},
		{"go install of a module version", "", &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}}, "v0.1.0"},
		{"go run outside a checkout", "", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, "dev"},
		{"no build info", "", nil, "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, describe(tt.stamped, tt.bi))
		})
	}
}
