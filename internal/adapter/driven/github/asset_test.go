package github_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/adapter/driven/github"
	"github.com/dturcinovic/seam/internal/domain/release"
)

// releaseServer answers for release v0.2.0 with files as its assets, which
// it serves under /download.
func releaseServer(t *testing.T, files map[string]string) *github.Client {
	t.Helper()
	return serve(t, func(w http.ResponseWriter, r *http.Request) {
		if name, ok := strings.CutPrefix(r.URL.Path, "/download/"); ok {
			_, _ = w.Write([]byte(files[name]))
			return
		}
		if r.URL.Path != "/repos/dturcinovic/seam/releases/tags/v0.2.0" {
			http.NotFound(w, r)
			return
		}
		var assets []string
		for name := range files {
			assets = append(assets, `{"name":"`+name+`","browser_download_url":"http://`+r.Host+`/download/`+name+`"}`)
		}
		_, _ = w.Write([]byte(`{"tag_name":"v0.2.0","assets":[` + strings.Join(assets, ",") + `]}`))
	})
}

func TestAnAssetIsDownloadedWhereTheReleaseSays(t *testing.T) {
	client := releaseServer(t, map[string]string{"checksums.txt": "abc  seam.tar.gz\n", "seam.tar.gz": "archive"})
	v, err := release.ParseVersion("v0.2.0")
	require.NoError(t, err)

	data, err := client.Asset(t.Context(), v, "seam.tar.gz", 100)
	require.NoError(t, err)
	assert.Equal(t, "archive", string(data))

	_, err = client.Asset(t.Context(), v, "seam.tar.gz", 3)
	require.ErrorContains(t, err, "larger than 3 bytes")
	_, err = client.Asset(t.Context(), v, "other.tar.gz", 100)
	require.ErrorContains(t, err, "not in release v0.2.0")

	missing, err := release.ParseVersion("v9.0.0")
	require.NoError(t, err)
	_, err = client.Asset(t.Context(), missing, "seam.tar.gz", 100)
	require.ErrorContains(t, err, "404 Not Found")
}
