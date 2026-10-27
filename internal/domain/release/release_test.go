package release_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/dturcinovic/seam/internal/domain/release"
)

func version(t *testing.T, s string) release.Version {
	t.Helper()
	v, err := release.ParseVersion(s)
	require.NoError(t, err)
	return v
}

func TestParseVersion(t *testing.T) {
	assert.Equal(t, "v1.20.3", version(t, "v1.20.3").String())
	assert.Equal(t, "v0.0.1", version(t, "v0.0.1").String())

	for _, bad := range []string{
		"", "dev", "v0.0.0", "1.2.3", "v1.2", "v1.2.3.4", "v1.2.x", "v01.2.3", "v1.2.-3", "v+1.2.3",
		"v0.1.0-12-g3164b9b", "v0.2.0-rc1", "v1.2.3 ",
	} {
		_, err := release.ParseVersion(bad)
		assert.ErrorIs(t, err, release.ErrNoRelease, "%q", bad)
	}
	assert.True(t, release.Version{}.IsZero())
	assert.Empty(t, release.Version{}.String())
}

func TestVersionsAreOrdered(t *testing.T) {
	part := rapid.IntRange(0, 30)
	draw := func(t *rapid.T, name string) release.Version {
		v, err := release.ParseVersion(fmt.Sprintf("v%d.%d.%d", part.Draw(t, name+".major"), part.Draw(t, name+".minor"), part.Draw(t, name+".patch")))
		if err != nil {
			t.Skip("v0.0.0 is no release")
		}
		return v
	}
	rapid.Check(t, func(t *rapid.T) {
		a, b, c := draw(t, "a"), draw(t, "b"), draw(t, "c")
		if again, err := release.ParseVersion(a.String()); err != nil || again != a {
			t.Fatalf("%s does not read back: %v", a, err)
		}
		if a.Newer(a) {
			t.Fatalf("%s is newer than itself", a)
		}
		if a != b && a.Newer(b) == b.Newer(a) {
			t.Fatalf("%s and %s are not ordered", a, b)
		}
		if a.Newer(b) && b.Newer(c) && !a.Newer(c) {
			t.Fatalf("%s > %s > %s but not %s > %s", a, b, c, a, c)
		}
	})
	assert.True(t, version(t, "v0.10.0").Newer(version(t, "v0.9.9")), "numbers, not text")
}

func TestCheckDue(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		at   time.Time
		want bool
	}{
		{"never asked", time.Time{}, true},
		{"just asked", now.Add(-time.Minute), false},
		{"a day ago", now.Add(-release.Interval), true},
		{"in the future", now.Add(time.Hour), true},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, release.Check{At: tt.at}.Due(now), tt.name)
	}
}

func TestNotice(t *testing.T) {
	assert.Equal(t, "seam v0.2.0 is out (you have v0.1.0): brew upgrade seam",
		release.Notice(version(t, "v0.1.0"), version(t, "v0.2.0"), "brew upgrade seam"))
}
