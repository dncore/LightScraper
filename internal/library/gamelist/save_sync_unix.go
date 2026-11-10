//go:build linux || darwin

package gamelist

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		if dirSyncUnsupported(err) {
			return fmt.Errorf("%w: %v", ErrDirSyncUnsupported, err)
		}
		return err
	}
	return dir.Close()
}

func dirSyncUnsupported(err error) bool {
	return errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP)
}
