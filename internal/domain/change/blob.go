package change

import (
	"crypto/sha1" //nolint:gosec // git names objects by SHA-1; it is no protection here.
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"strconv"
)

// ObjectFormat is the hash a repository names its objects by.
type ObjectFormat uint8

// The object formats. The zero value is no format.
const (
	_ ObjectFormat = iota
	SHA1
	SHA256
)

// ErrUnknownObjectFormat is a format git names that seam does not know.
var ErrUnknownObjectFormat = errors.New("unknown object format")

// ParseObjectFormat reads the format as git names it.
func ParseObjectFormat(s string) (ObjectFormat, error) {
	switch s {
	case "sha1":
		return SHA1, nil
	case "sha256":
		return SHA256, nil
	}
	return 0, fmt.Errorf("%w: %q", ErrUnknownObjectFormat, s)
}

// BlobID is the id git gives a blob of data in format f, as git
// hash-object would, without writing it; zero for no format.
func BlobID(f ObjectFormat, data []byte) ObjectID {
	var h hash.Hash
	switch f {
	case SHA1:
		h = sha1.New() //nolint:gosec // see the import
	case SHA256:
		h = sha256.New()
	default:
		return ObjectID{}
	}
	h.Write([]byte("blob " + strconv.Itoa(len(data)) + "\x00"))
	h.Write(data)
	return ObjectID{hex: hex.EncodeToString(h.Sum(nil))}
}
