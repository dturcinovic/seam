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

// Client asks for the latest release and downloads the files of one.
type Client struct {
	http  *http.Client
	repo  string
	agent string
}

// New returns a client for c. It keeps no connection open between
// questions: there is one a day, and an idle connection would leave a
// goroutine of net/http behind.
func New(c Config) *Client {
	return &Client{
		http:  &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DisableKeepAlives: true}},
		repo:  strings.TrimSuffix(cmp.Or(c.BaseURL, defaultBaseURL), "/") + "/repos/" + c.Repo,
		agent: c.Agent,
	}
}

// LatestRelease implements out.ReleaseFetcher.
func (c *Client) LatestRelease(ctx context.Context) (release.Version, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := c.getJSON(ctx, c.repo+"/releases/latest", &body); err != nil {
		return release.Version{}, fmt.Errorf("latest release: %w", err)
	}
	v, err := release.ParseVersion(body.TagName)
	if err != nil {
		return release.Version{}, fmt.Errorf("latest release: %w", err)
	}
	return v, nil
}

// getJSON asks the API at url and decodes its answer into body.
func (c *Client) getJSON(ctx context.Context, url string, body any) error {
	resp, err := c.get(ctx, url, "application/vnd.github+json")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }() // read only: closing cannot lose data
	return json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(body)
}

// get asks url for accept, as seam and without anyone's credentials; any
// answer but 200 is an error.
func (c *Client) get(ctx context.Context, url, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", c.agent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close() // read only: closing cannot lose data
		return nil, fmt.Errorf("%s", resp.Status)
	}
	return resp, nil
}
