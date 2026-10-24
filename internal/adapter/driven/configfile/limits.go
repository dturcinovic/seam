package configfile

import (
	"fmt"

	"seam/internal/domain/limits"
)

type limitsEntry struct {
	MaxFileSize  *string `toml:"max_file_size"`
	MaxInstances *int    `toml:"max_instances"`
}

// limits are the limits the section sets, zero for those it does not.
func (e limitsEntry) limits() (size limits.FileSize, instances limits.Instances, err error) {
	if e.MaxFileSize != nil {
		if size, err = limits.ParseFileSize(*e.MaxFileSize); err != nil {
			return 0, 0, fmt.Errorf("limits.max_file_size: %w", err)
		}
	}
	if e.MaxInstances != nil {
		if instances, err = limits.NewInstances(*e.MaxInstances); err != nil {
			return 0, 0, fmt.Errorf("limits.max_instances: %w", err)
		}
	}
	return size, instances, nil
}
