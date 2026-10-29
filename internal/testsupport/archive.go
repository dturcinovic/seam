package testsupport

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/stretchr/testify/require"
)

// TarGz is a gzipped tar of files, given as name and content in turn, as
// goreleaser packs a release archive.
func TarGz(t *testing.T, files ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for i := 0; i+1 < len(files); i += 2 {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: files[i], Mode: 0o755, Size: int64(len(files[i+1])), Typeflag: tar.TypeReg}))
		_, err := tw.Write([]byte(files[i+1]))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}
