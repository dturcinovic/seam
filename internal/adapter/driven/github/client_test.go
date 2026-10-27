package github_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/dturcinovic/seam/internal/adapter/driven/github"
	"github.com/dturcinovic/seam/internal/domain/release"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func serve(t *testing.T, handler http.HandlerFunc) *github.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return github.New(github.Config{BaseURL: srv.URL + "/", Repo: "dturcinovic/seam", Agent: "seam/v0.1.0"})
}

func TestTheLatestRelease(t *testing.T) {
	var got *http.Request
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		got = r
		_, _ = w.Write([]byte(`{"tag_name":"v0.2.0","name":"seam v0.2.0","assets":[]}`))
	})

	v, err := client.LatestRelease(t.Context())

	require.NoError(t, err)
	assert.Equal(t, "v0.2.0", v.String())
	assert.Equal(t, "/repos/dturcinovic/seam/releases/latest", got.URL.Path)
	assert.Equal(t, "seam/v0.1.0", got.Header.Get("User-Agent"))
	assert.Equal(t, "application/vnd.github+json", got.Header.Get("Accept"))
	assert.Empty(t, got.Header.Get("Authorization"), "the API is asked without anyone's credentials")
}

func TestAnswersThatNameNoRelease(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    error
	}{
		{"private or no release yet", func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }, nil},
		{"rate limited", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }, nil},
		{"no JSON", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>")) }, nil},
		{"a tag that is no release", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"tag_name":"nightly"}`))
		}, release.ErrNoRelease},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := serve(t, tt.handler).LatestRelease(t.Context())
			require.Error(t, err)
			if tt.want != nil {
				assert.ErrorIs(t, err, tt.want)
			}
		})
	}
}

func TestACancelledQuestionReturnsAtOnce(t *testing.T) {
	unblock := make(chan struct{})
	client := serve(t, func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-unblock:
		case <-r.Context().Done():
		}
	})
	defer close(unblock)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := client.LatestRelease(ctx)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), time.Second)
}

func TestNoServerIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()

	_, err := github.New(github.Config{BaseURL: srv.URL}).LatestRelease(t.Context())

	assert.Error(t, err)
}
