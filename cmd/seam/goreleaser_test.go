package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seam upgrade finds a release's files by the names release.ArchiveName
// and release.PackageCommands give them; this holds .goreleaser.yaml to
// those names.
func TestTheReleaseNamesItsFilesAsSeamUpgradeLooksForThem(t *testing.T) {
	data, err := os.ReadFile("../../.goreleaser.yaml")
	require.NoError(t, err)
	cfg := string(data)
	assert.Contains(t, cfg, `name_template: '{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}'`, "release.ArchiveName")
	assert.Contains(t, cfg, "formats: [tar.gz]", "release.ArchiveName")
	assert.Contains(t, cfg, `file_name_template: '{{ .ConventionalFileName }}'`, "release.PackageCommands")
	assert.Contains(t, cfg, "name_template: checksums.txt", "release.ChecksumsName")
}
