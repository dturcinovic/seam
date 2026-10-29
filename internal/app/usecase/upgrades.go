package usecase

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/dturcinovic/seam/internal/app/port/out"
	"github.com/dturcinovic/seam/internal/domain/release"
)

// maxChecksums bounds checksums.txt, a line per file of a release.
const maxChecksums = 64 << 10

// UpgradeSettings is the install seam upgrade works on: the running
// release, how it was installed and where its binary lies, the module
// go install builds and the system it runs on.
type UpgradeSettings struct {
	Running        release.Version
	Install        release.Install
	Binary, Module string
	GOOS, GOARCH   string
}

// Upgrades implements in.Upgrader. It replaces seam's binary only for an
// archive: a package manager owns the others, which upgrade by their own
// command or by the user.
type Upgrades struct {
	set     UpgradeSettings
	fetch   out.ReleaseFetcher
	assets  out.ReleaseAssets
	replace out.BinaryReplacer
	run     out.CommandRunner
}

// NewUpgrades returns the use case over its ports.
func NewUpgrades(set UpgradeSettings, f out.ReleaseFetcher, a out.ReleaseAssets, b out.BinaryReplacer, r out.CommandRunner) *Upgrades {
	return &Upgrades{set: set, fetch: f, assets: a, replace: b, run: r}
}

// Upgrade implements in.Upgrader. When it fails after asking, the Upgrade
// it returns is ByHand and says how.
func (u *Upgrades) Upgrade(ctx context.Context, check bool) (release.Upgrade, error) {
	if u.set.Running.IsZero() {
		return release.Upgrade{}, release.ErrNotARelease
	}
	latest, err := u.fetch.LatestRelease(ctx)
	if err != nil {
		return release.Upgrade{}, err
	}
	up := release.Upgrade{Outcome: release.ByHand, Running: u.set.Running, Latest: latest, Binary: u.set.Binary}
	switch {
	case !latest.Newer(u.set.Running):
		up.Outcome = release.Current
		return up, nil
	case u.set.Install == release.Package:
		up.Outcome, up.Commands = release.ByPackage, release.PackageCommands(latest, u.set.GOARCH)
		return up, nil
	}
	if cmd, ok := u.set.Install.Command(u.set.Module, latest); ok {
		return u.runCommand(ctx, up, cmd, check)
	}
	if u.set.Install != release.Archive {
		return up, nil
	}
	if check {
		up.Outcome = release.WouldReplace
		return up, nil
	}
	if err := u.replaceArchive(ctx, latest); err != nil {
		// An archive in /usr/local/bin is mostly root's.
		if errors.Is(err, fs.ErrPermission) {
			up.Commands = []string{"sudo seam upgrade"}
		}
		return up, err
	}
	up.Outcome = release.Replaced
	return up, nil
}

// runCommand upgrades up by cmd, or with check only names it.
func (u *Upgrades) runCommand(ctx context.Context, up release.Upgrade, cmd []string, check bool) (release.Upgrade, error) {
	up.Commands = []string{strings.Join(cmd, " ")}
	if check {
		up.Outcome = release.WouldRun
		return up, nil
	}
	if err := u.run.Run(ctx, cmd[0], cmd[1:]...); err != nil {
		return up, fmt.Errorf("%s: %w", up.Commands[0], err)
	}
	up.Outcome = release.Ran
	return up, nil
}

// replaceArchive puts the binary of release v's archive in place of seam's
// own, once its checksum is the one the release names.
func (u *Upgrades) replaceArchive(ctx context.Context, v release.Version) error {
	name := release.ArchiveName(v, u.set.GOOS, u.set.GOARCH)
	sums, err := u.assets.Asset(ctx, v, release.ChecksumsName, maxChecksums)
	if err != nil {
		return fmt.Errorf("download %s: %w", release.ChecksumsName, err)
	}
	// The archive is smaller than the binary it holds.
	archive, err := u.assets.Asset(ctx, v, name, release.MaxBinary)
	if err != nil {
		return fmt.Errorf("download %s: %w", name, err)
	}
	if err := release.Verify(sums, name, archive); err != nil {
		return err
	}
	bin, err := release.ExtractBinary(archive)
	if err != nil {
		return err
	}
	if err := u.replace.Replace(ctx, u.set.Binary, bin); err != nil {
		return fmt.Errorf("replace %s: %w", u.set.Binary, err)
	}
	return nil
}
