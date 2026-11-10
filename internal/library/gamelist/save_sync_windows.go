//go:build windows

package gamelist

import (
	"errors"
	"fmt"
)

func syncDir(path string) error {
	return fmt.Errorf("%w: Windows standard library has no portable directory sync", ErrDirSyncUnsupported)
}

func dirSyncUnsupported(err error) bool {
	return errors.Is(err, ErrDirSyncUnsupported)
}
