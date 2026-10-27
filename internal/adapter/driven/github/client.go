package github

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dturcinovic/seam/internal/domain/release"
)

const (
	defaultBaseURL = "https://api.github.com"
	// timeout bounds one question: a slow network must not hold the
	// session's stop.
	timeout = 5 * time.Second
	// maxBody bounds what is read of an answer; a release is a few KiB.
	maxBody = 1 << 20
)

// Client asks for the latest release.
type Client struct {
	http  *http.Client
	url   string
	agent string
}

// New returns a client for c. It keeps no connection open between
// questions: there is one a day, and an idle connection would leave a
// goroutine of net/http behind.
func New(c Config) *Client {
	return &Client{
		http:  &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DisableKeepAlives: true}},
		url:   strings.TrimSuffix(cmp.Or(c.BaseURL, defaultBaseURL), "/") + "/repos/" + c.Repo + "/releases/latest",
		agent: c.Agent,
	}
}

// LatestRelease implements out.ReleaseFetcher.
func (c *Client) LatestRelease(ctx context.Context) (release.Version, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, http.NoBody)
	if err != nil {
		return release.Version{}, fmt.Errorf("latest release: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", c.agent)
	resp, err := c.http.Do(req)
	if err != nil {
		return release.Version{}, fmt.Errorf("latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }() // read only: closing cannot lose data
	if resp.StatusCode != http.StatusOK {
		return release.Version{}, fmt.Errorf("latest release: %s", resp.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&body); err != nil {
		return release.Version{}, fmt.Errorf("latest release: %w", err)
	}
	v, err := release.ParseVersion(body.TagName)
	if err != nil {
		return release.Version{}, fmt.Errorf("latest release: %w", err)
	}
	return v, nil
}
