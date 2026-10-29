package usecase_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/app/port/out/outfakes"
	"github.com/dturcinovic/seam/internal/app/usecase"
	"github.com/dturcinovic/seam/internal/domain/release"
	"github.com/dturcinovic/seam/internal/testsupport"
)

const (
	upgradeModule = "github.com/dturcinovic/seam/cmd/seam"
	newBinary     = "\x7fELF v0.2.0"
)

type upgradeFakes struct {
	fetch   *outfakes.FakeReleaseFetcher
	assets  *outfakes.FakeReleaseAssets
	replace *outfakes.FakeBinaryReplacer
	run     *outfakes.FakeCommandRunner
}

// newUpgrades upgrades a v0.1.0 of kind install on linux/amd64, whose
// latest release is v0.2.0 with an archive and checksums that match.
func newUpgrades(t *testing.T, install release.Install) (*usecase.Upgrades, upgradeFakes) {
	t.Helper()
	f := upgradeFakes{&outfakes.FakeReleaseFetcher{}, &outfakes.FakeReleaseAssets{}, &outfakes.FakeBinaryReplacer{}, &outfakes.FakeCommandRunner{}}
	f.fetch.LatestReleaseReturns(releaseVersion(t, "v0.2.0"), nil)
	archive := testsupport.TarGz(t, "seam", newBinary)
	sum := sha256.Sum256(archive)
	files := map[string][]byte{
		"checksums.txt":                 []byte(hex.EncodeToString(sum[:]) + "  seam_0.2.0_linux_amd64.tar.gz\n"),
		"seam_0.2.0_linux_amd64.tar.gz": archive,
	}
	f.assets.AssetCalls(func(_ context.Context, v release.Version, name string, _ int64) ([]byte, error) {
		assert.Equal(t, "v0.2.0", v.String())
		data, ok := files[name]
		if !ok {
			return nil, errors.New("404 Not Found")
		}
		return data, nil
	})
	set := usecase.UpgradeSettings{
		Running: releaseVersion(t, "v0.1.0"), Install: install, Binary: "/opt/seam/seam", Module: upgradeModule,
		GOOS: "linux", GOARCH: "amd64",
	}
	return usecase.NewUpgrades(set, f.fetch, f.assets, f.replace, f.run), f
}

func TestAnArchiveReplacesItsBinary(t *testing.T) {
	u, f := newUpgrades(t, release.Archive)
	up, err := u.Upgrade(t.Context(), false)
	require.NoError(t, err)
	assert.Equal(t, release.Replaced, up.Outcome)
	require.Equal(t, 1, f.replace.ReplaceCallCount())
	_, path, data := f.replace.ReplaceArgsForCall(0)
	assert.Equal(t, "/opt/seam/seam", path)
	assert.Equal(t, []byte(newBinary), data)
}

func TestAnArchiveThatDoesNotCheckOutIsNotPutInPlace(t *testing.T) {
	u, f := newUpgrades(t, release.Archive)
	inner := f.assets.AssetStub
	f.assets.AssetStub = func(ctx context.Context, v release.Version, name string, max int64) ([]byte, error) {
		if name == "seam_0.2.0_linux_amd64.tar.gz" {
			return []byte("tampered"), nil
		}
		return inner(ctx, v, name, max)
	}
	up, err := u.Upgrade(t.Context(), false)
	require.ErrorIs(t, err, release.ErrChecksum)
	assert.Equal(t, release.ByHand, up.Outcome)
	assert.Empty(t, up.Commands, "from the release page")
	assert.Zero(t, f.replace.ReplaceCallCount())
}

func TestADownloadOrReplaceThatFailsLeavesItToTheUser(t *testing.T) {
	u, f := newUpgrades(t, release.Archive)
	f.replace.ReplaceReturns(&fs.PathError{Op: "open", Path: "/opt/seam/.seam.upgrade-1", Err: fs.ErrPermission})
	up, err := u.Upgrade(t.Context(), false)
	require.ErrorIs(t, err, fs.ErrPermission)
	assert.Equal(t, release.ByHand, up.Outcome)
	assert.Equal(t, []string{"sudo seam upgrade"}, up.Commands, "a directory of root's")

	u, f = newUpgrades(t, release.Archive)
	f.assets.AssetCalls(func(context.Context, release.Version, string, int64) ([]byte, error) {
		return nil, errors.New("unexpected EOF")
	})
	_, err = u.Upgrade(t.Context(), false)
	require.ErrorContains(t, err, "download checksums.txt: unexpected EOF")
	assert.Zero(t, f.replace.ReplaceCallCount())
}

func TestAnInstallWithItsOwnCommandRunsIt(t *testing.T) {
	tests := map[release.Install][]string{
		release.Homebrew:  {"brew", "upgrade", "seam"},
		release.GoInstall: {"go", "install", upgradeModule + "@v0.2.0"},
	}
	for install, cmd := range tests {
		u, f := newUpgrades(t, install)
		up, err := u.Upgrade(t.Context(), false)
		require.NoError(t, err)
		assert.Equal(t, release.Ran, up.Outcome)
		require.Equal(t, 1, f.run.RunCallCount())
		_, name, args := f.run.RunArgsForCall(0)
		assert.Equal(t, cmd, append([]string{name}, args...))
		assert.Zero(t, f.replace.ReplaceCallCount())

		f.run.RunReturns(errors.New("exit status 1"))
		up, err = u.Upgrade(t.Context(), false)
		require.ErrorContains(t, err, "exit status 1")
		assert.Equal(t, release.ByHand, up.Outcome)
		assert.Equal(t, []string{strings.Join(cmd, " ")}, up.Commands, "to run again")
	}
}

func TestAPackageIsNeverReplaced(t *testing.T) {
	u, f := newUpgrades(t, release.Package)
	up, err := u.Upgrade(t.Context(), false)
	require.NoError(t, err)
	assert.Equal(t, release.ByPackage, up.Outcome)
	assert.Equal(t, []string{"sudo apt install ./seam_0.2.0_amd64.deb", "sudo dnf install ./seam-0.2.0-1.x86_64.rpm"}, up.Commands)
	assert.Zero(t, f.replace.ReplaceCallCount())
	assert.Zero(t, f.run.RunCallCount())
	assert.Zero(t, f.assets.AssetCallCount())
}

func TestCheckAndCurrentChangeNothing(t *testing.T) {
	checked := map[release.Install]release.Outcome{
		release.Homebrew: release.WouldRun, release.GoInstall: release.WouldRun, release.Package: release.ByPackage,
		release.Archive: release.WouldReplace, 0: release.ByHand,
	}
	for install, want := range checked {
		u, f := newUpgrades(t, install)
		up, err := u.Upgrade(t.Context(), true)
		require.NoError(t, err)
		assert.Equal(t, want, up.Outcome, "install %d", install)
		assert.Zero(t, f.run.RunCallCount()+f.replace.ReplaceCallCount()+f.assets.AssetCallCount())

		f.fetch.LatestReleaseReturns(releaseVersion(t, "v0.1.0"), nil)
		up, err = u.Upgrade(t.Context(), false)
		require.NoError(t, err)
		assert.Equal(t, release.Current, up.Outcome)
		assert.Zero(t, f.run.RunCallCount()+f.replace.ReplaceCallCount()+f.assets.AssetCallCount())
	}
}

func TestABuildThatIsNoReleaseIsNotUpgraded(t *testing.T) {
	f := upgradeFakes{&outfakes.FakeReleaseFetcher{}, &outfakes.FakeReleaseAssets{}, &outfakes.FakeBinaryReplacer{}, &outfakes.FakeCommandRunner{}}
	u := usecase.NewUpgrades(usecase.UpgradeSettings{Install: release.Archive}, f.fetch, f.assets, f.replace, f.run)
	_, err := u.Upgrade(t.Context(), false)
	require.ErrorIs(t, err, release.ErrNotARelease)
	assert.Zero(t, f.fetch.LatestReleaseCallCount(), "it asks no one")
}
