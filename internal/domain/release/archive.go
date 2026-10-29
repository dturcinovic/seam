package release

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Errors of an archive downloaded.
var (
	// ErrChecksum is an archive whose SHA-256 is not the one checksums.txt
	// names, or that it does not name at all.
	ErrChecksum = errors.New("checksum does not match")
	// ErrNoBinary is an archive without a binary named seam in it.
	ErrNoBinary = errors.New("no seam in the archive")
)

// MaxBinary bounds the binary an archive may hold; seam is some 25 MiB.
const MaxBinary = 128 << 20

// ChecksumsName is the file of a release that names each file's SHA-256.
const ChecksumsName = "checksums.txt"

// ArchiveName is the archive of release v for goos and goarch, as
// name_template in .goreleaser.yaml names it: seam_0.2.0_linux_amd64.tar.gz.
func ArchiveName(v Version, goos, goarch string) string {
	return "seam_" + strings.TrimPrefix(v.String(), "v") + "_" + goos + "_" + goarch + ".tar.gz"
}

// Verify checks data, the file name of a release, against checksums, the
// release's checksums.txt: lines of a hex SHA-256 and a name.
func Verify(checksums []byte, name string, data []byte) error {
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	sc := bufio.NewScanner(bytes.NewReader(checksums))
	for sc.Scan() {
		want, file, ok := strings.Cut(strings.TrimSpace(sc.Text()), "  ")
		if ok && file == name {
			if want != got {
				return fmt.Errorf("%w: %s", ErrChecksum, name)
			}
			return nil
		}
	}
	return fmt.Errorf("%w: %s not in %s", ErrChecksum, name, ChecksumsName)
}

// ExtractBinary is the file named seam at the top of archive, a gzipped
// tar as goreleaser builds it, of at most MaxBinary bytes.
func ExtractBinary(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, ErrNoBinary
		}
		if err != nil {
			return nil, fmt.Errorf("read archive: %w", err)
		}
		if h.Name != "seam" || h.Typeflag != tar.TypeReg {
			continue
		}
		if h.Size > MaxBinary {
			return nil, fmt.Errorf("read archive: seam is %d bytes, at most %d", h.Size, MaxBinary)
		}
		bin := make([]byte, h.Size)
		if _, err := io.ReadFull(tr, bin); err != nil {
			return nil, fmt.Errorf("read archive: %w", err)
		}
		return bin, nil
	}
}
