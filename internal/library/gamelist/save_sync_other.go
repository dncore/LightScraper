//go:build !linux && !darwin && !windows

package gamelist

import (
	"errors"
	"fmt"
)

func syncDir(path string) error {
	return fmt.Errorf("%w: directory sync is not implemented for this platform", ErrDirSyncUnsupported)
}

func dirSyncUnsupported(err error) bool {
	return errors.Is(err, ErrDirSyncUnsupported)
}
