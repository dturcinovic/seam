package github

import (
	"context"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/dturcinovic/seam/internal/domain/release"
)

// downloadTimeout bounds one file of a release: seam upgrade waits for it,
// and an archive is some 10 MiB.
const downloadTimeout = 2 * time.Minute

type assetJSON struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// Asset implements out.ReleaseAssets. It finds the file among the assets
// of release v and downloads it where the API says it lies, which a test
// server or mirror names as it likes.
func (c *Client) Asset(ctx context.Context, v release.Version, name string, maxBytes int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	var body struct {
		Assets []assetJSON `json:"assets"`
	}
	if err := c.getJSON(ctx, c.repo+"/releases/tags/"+v.String(), &body); err != nil {
		return nil, fmt.Errorf("release %s: %w", v, err)
	}
	i := slices.IndexFunc(body.Assets, func(a assetJSON) bool { return a.Name == name })
	if i < 0 {
		return nil, fmt.Errorf("not in release %s", v)
	}
	resp, err := c.get(ctx, body.Assets[i].URL, "application/octet-stream")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }() // read only: closing cannot lose data
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("larger than %d bytes", maxBytes)
	}
	return data, nil
}
