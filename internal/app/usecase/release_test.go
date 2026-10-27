package usecase_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/app/port/out/outfakes"
	"github.com/dturcinovic/seam/internal/app/usecase"
	"github.com/dturcinovic/seam/internal/domain/release"
)

// memStore keeps the check in memory, as the state file does between
// sessions; like it, it saves nothing once the session has stopped.
type memStore struct {
	mu    sync.Mutex
	check release.Check
	err   error
	saves int
}

func (s *memStore) LoadCheck(context.Context) (release.Check, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return release.Check{}, s.err
	}
	return s.check, nil
}

func (s *memStore) SaveCheck(ctx context.Context, c release.Check) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.check, s.err = c, nil
	s.saves++
	return nil
}

func (s *memStore) saved() release.Check {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.check
}

func releaseVersion(t *testing.T, s string) release.Version {
	t.Helper()
	v, err := release.ParseVersion(s)
	require.NoError(t, err)
	return v
}

// latestIs is a fetcher that answers v0.2.0.
func latestIs(t *testing.T) *outfakes.FakeReleaseFetcher {
	t.Helper()
	f := &outfakes.FakeReleaseFetcher{}
	f.LatestReleaseReturns(releaseVersion(t, "v0.2.0"), nil)
	return f
}

// runSession is one run of seam: it checks, waits for the check, asks for
// a newer release and stops.
func runSession(t *testing.T, s usecase.ReleaseSettings, f *outfakes.FakeReleaseFetcher, store *memStore) (release.Version, bool) {
	t.Helper()
	uc := usecase.NewReleaseChecks(s, f, store, nil)
	uc.Check()
	synctest.Wait()
	v, ok := uc.Newer(t.Context())
	require.NoError(t, uc.Stop(t.Context()))
	return v, ok
}

func TestANewerReleaseIsAskedForOnceADay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f, store := latestIs(t), &memStore{}
		s := usecase.ReleaseSettings{Running: releaseVersion(t, "v0.1.0"), Ask: true}

		v, ok := runSession(t, s, f, store)
		assert.True(t, ok)
		assert.Equal(t, "v0.2.0", v.String())

		time.Sleep(release.Interval - time.Minute)
		_, ok = runSession(t, s, f, store)
		assert.Equal(t, 1, f.LatestReleaseCallCount(), "the same day asks no more")
		assert.True(t, ok, "the answer comes from the record")

		time.Sleep(time.Minute)
		runSession(t, s, f, store)
		assert.Equal(t, 2, f.LatestReleaseCallCount(), "a day later asks again")
	})
}

func TestTheSameOrAnOlderReleaseIsNotNewer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, running := range []string{"v0.2.0", "v0.3.0"} {
			_, ok := runSession(t, usecase.ReleaseSettings{Running: releaseVersion(t, running), Ask: true}, latestIs(t), &memStore{})
			assert.False(t, ok, running)
		}
	})
}

func TestAFailedQuestionCountsAsTheDaysOne(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f, store := latestIs(t), &memStore{}
		f.LatestReleaseReturns(release.Version{}, errors.New("no network"))
		s := usecase.ReleaseSettings{Running: releaseVersion(t, "v0.1.0"), Ask: true}

		_, ok := runSession(t, s, f, store)
		assert.False(t, ok, "a failed check finds nothing")
		runSession(t, s, f, store)

		assert.Equal(t, 1, f.LatestReleaseCallCount())
		assert.Equal(t, time.Now(), store.saved().At)
	})
}

func TestStopCancelsAQuestionThatHangs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f, store := latestIs(t), &memStore{}
		f.LatestReleaseCalls(func(ctx context.Context) (release.Version, error) {
			<-ctx.Done()
			return release.Version{}, ctx.Err()
		})
		start := time.Now()

		_, ok := runSession(t, usecase.ReleaseSettings{Running: releaseVersion(t, "v0.1.0"), Ask: true}, f, store)

		assert.False(t, ok)
		assert.Equal(t, start, time.Now(), "stopping waits for nothing")
		assert.Equal(t, start, store.saved().At, "the attempt is recorded before asking")
	})
}

func TestNothingIsAskedWhenSeamMayNot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tests := []struct {
			name     string
			settings usecase.ReleaseSettings
		}{
			{"asking is off", usecase.ReleaseSettings{Running: releaseVersion(t, "v0.1.0")}},
			{"a build that is no release", usecase.ReleaseSettings{Ask: true}},
		}
		for _, tt := range tests {
			f := latestIs(t)
			store := &memStore{check: release.Check{Latest: releaseVersion(t, "v0.2.0"), At: time.Now().Add(-48 * time.Hour)}}

			_, ok := runSession(t, tt.settings, f, store)

			assert.False(t, ok, "%s: no answer from an old record either", tt.name)
			assert.Zero(t, f.LatestReleaseCallCount(), tt.name)
			assert.Zero(t, store.saves, tt.name)
		}
	})
}

func TestABrokenRecordCountsAsNone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f, store := latestIs(t), &memStore{err: errors.New("broken")}

		v, ok := runSession(t, usecase.ReleaseSettings{Running: releaseVersion(t, "v0.1.0"), Ask: true}, f, store)

		assert.Equal(t, 1, f.LatestReleaseCallCount())
		assert.True(t, ok, "asking replaced it")
		assert.Equal(t, "v0.2.0", v.String())
	})
}

func TestNewerReleaseReadsOnly(t *testing.T) {
	store := &memStore{check: release.Check{Latest: releaseVersion(t, "v0.2.0")}}

	v, ok := usecase.NewerRelease(t.Context(), usecase.ReleaseSettings{Running: releaseVersion(t, "v0.1.0"), Ask: true}, store)

	assert.True(t, ok)
	assert.Equal(t, "v0.2.0", v.String())
	assert.Zero(t, store.saves)
}
