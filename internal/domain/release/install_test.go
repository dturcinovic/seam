package release_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/release"
	"github.com/dturcinovic/seam/internal/testsupport"
)

func TestTheCommandOfAnInstall(t *testing.T) {
	const module = "github.com/dturcinovic/seam/cmd/seam"
	const page = "https://github.com/dturcinovic/seam/releases/latest"
	tests := []struct {
		install release.Install
		module  string
		cmd     []string
		hint    string
	}{
		{release.Homebrew, module, []string{"brew", "upgrade", "seam"}, "brew upgrade seam"},
		{release.GoInstall, module, []string{"go", "install", module + "@v0.2.0"}, "go install " + module + "@v0.2.0"},
		{release.GoInstall, "", nil, page},
		{release.Package, module, nil, page},
		{release.Archive, module, nil, "seam upgrade"},
		{0, module, nil, page},
	}
	for _, tt := range tests {
		cmd, ok := tt.install.Command(tt.module, version(t, "v0.2.0"))
		assert.Equal(t, tt.cmd, cmd)
		assert.Equal(t, tt.cmd != nil, ok)
		assert.Equal(t, tt.hint, tt.install.Hint(tt.module, page, version(t, "v0.2.0")))
	}
}

func TestGoInstallGetsTheReleaseFound(t *testing.T) {
	cmd, ok := release.GoInstall.Command("example.com/seam", version(t, "v0.2.0"))
	require.True(t, ok)
	assert.Equal(t, []string{"go", "install", "example.com/seam@v0.2.0"}, cmd)
}

func TestPackagesAreNamedAsGoreleaserNamesThem(t *testing.T) {
	assert.Equal(t, []string{"sudo apt install ./seam_0.2.0_amd64.deb", "sudo dnf install ./seam-0.2.0-1.x86_64.rpm"},
		release.PackageCommands(version(t, "v0.2.0"), "amd64"))
	assert.Equal(t, []string{"sudo apt install ./seam_1.0.3_arm64.deb", "sudo dnf install ./seam-1.0.3-1.aarch64.rpm"},
		release.PackageCommands(version(t, "v1.0.3"), "arm64"))
	assert.Equal(t, "seam_0.2.0_darwin_arm64.tar.gz", release.ArchiveName(version(t, "v0.2.0"), "darwin", "arm64"))
}

func TestTheBinaryIsTakenFromTheArchive(t *testing.T) {
	bin, err := release.ExtractBinary(testsupport.TarGz(t, "README.md", "read me", "docs/seam", "not it", "seam", "\x7fELF"))
	require.NoError(t, err)
	assert.Equal(t, []byte("\x7fELF"), bin)

	_, err = release.ExtractBinary(testsupport.TarGz(t, "README.md", "read me"))
	require.ErrorIs(t, err, release.ErrNoBinary)
	_, err = release.ExtractBinary([]byte("no gzip"))
	require.Error(t, err)
}

func TestAnArchiveIsCheckedAgainstTheChecksums(t *testing.T) {
	data := []byte("archive")
	sum := sha256.Sum256(data)
	checksums := []byte("0000  other.tar.gz\n" + hex.EncodeToString(sum[:]) + "  seam_0.2.0_linux_amd64.tar.gz\n")

	require.NoError(t, release.Verify(checksums, "seam_0.2.0_linux_amd64.tar.gz", data))
	require.ErrorIs(t, release.Verify(checksums, "seam_0.2.0_linux_amd64.tar.gz", []byte("changed")), release.ErrChecksum)
	require.ErrorIs(t, release.Verify(checksums, "seam_0.2.0_linux_arm64.tar.gz", data), release.ErrChecksum, "a file it does not name")
}
