package gamelist

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
)

// stableRead is one verified read from an opened file handle.
type stableRead struct {
	data []byte
	info os.FileInfo
	hash [32]byte
}

// readStableFile opens path, reads through the handle, and verifies before/after
// path identity and file metadata. expected, when non-nil, must identify the
// same file that was opened. It never writes.
func readStableFile(path string, expected os.FileInfo) (stableRead, error) {
	var result stableRead

	pathBefore, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return result, fmt.Errorf("%w: %s", ErrSourceMissing, path)
		}
		if os.IsPermission(err) {
			return result, fmt.Errorf("%w: %s", ErrSourcePermission, path)
		}
		return result, fmt.Errorf("gamelist: stat %s: %w", path, err)
	}
	if pathBefore.Mode()&os.ModeSymlink != 0 {
		return result, fmt.Errorf("%w: %s", ErrSymlink, path)
	}
	if !pathBefore.Mode().IsRegular() {
		return result, fmt.Errorf("%w: %s", ErrNotRegular, path)
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return result, fmt.Errorf("%w: %s", ErrSourceMissing, path)
		}
		if os.IsPermission(err) {
			return result, fmt.Errorf("%w: %s", ErrSourcePermission, path)
		}
		return result, fmt.Errorf("gamelist: open %s: %w", path, err)
	}
	defer file.Close()

	openedInfo, err := file.Stat()
	if err != nil {
		return result, fmt.Errorf("gamelist: stat opened file: %w", err)
	}
	if !openedInfo.Mode().IsRegular() {
		return result, fmt.Errorf("%w: %s", ErrNotRegular, path)
	}
	pathOpened, err := os.Lstat(path)
	if err != nil {
		return result, fmt.Errorf("%w: %s: %v", ErrSourceReplaced, path, err)
	}
	if pathOpened.Mode()&os.ModeSymlink != 0 || !pathOpened.Mode().IsRegular() || !os.SameFile(pathOpened, openedInfo) {
		return result, fmt.Errorf("%w: %s: path changed after open", ErrSourceReplaced, path)
	}

	data, err := io.ReadAll(file)
	if err != nil {
		if os.IsPermission(err) {
			return result, fmt.Errorf("%w: %s", ErrSourcePermission, path)
		}
		return result, fmt.Errorf("gamelist: read %s: %w", path, err)
	}
	afterInfo, err := file.Stat()
	if err != nil {
		return result, fmt.Errorf("gamelist: stat opened file after read: %w", err)
	}
	if !os.SameFile(afterInfo, openedInfo) ||
		afterInfo.Size() != int64(len(data)) ||
		afterInfo.Mode() != openedInfo.Mode() ||
		!afterInfo.ModTime().Equal(openedInfo.ModTime()) {
		return result, fmt.Errorf("%w: %s: file changed while reading", ErrSourceChanged, path)
	}

	pathAfter, err := os.Lstat(path)
	if err != nil {
		return result, fmt.Errorf("%w: %s: %v", ErrSourceReplaced, path, err)
	}
	if pathAfter.Mode()&os.ModeSymlink != 0 || !pathAfter.Mode().IsRegular() || !os.SameFile(pathAfter, afterInfo) {
		return result, fmt.Errorf("%w: %s: path changed during read", ErrSourceReplaced, path)
	}
	if expected != nil && !os.SameFile(afterInfo, expected) {
		return result, fmt.Errorf("%w: %s", ErrSourceReplaced, path)
	}

	result.data = data
	result.info = afterInfo
	result.hash = sha256.Sum256(data)
	return result, nil
}

type targetState uint8

const (
	targetUnknown targetState = iota
	targetOld
	targetNew
	targetOther
	targetMissing
	targetSymlink
	targetNonRegular
	targetUnreadable
)

// inspectTarget determines what currently exists at path without writing.
// expectedOld and expectedNew are optional file identities used to distinguish
// the pre-existing target from the temporary file that was renamed.
func inspectTarget(path string, oldInfo, newInfo os.FileInfo, newBytes []byte, oldHash [32]byte) targetState {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return targetMissing
		}
		return targetUnknown
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return targetSymlink
	}
	if !info.Mode().IsRegular() {
		return targetNonRegular
	}

	current, err := readStableFile(path, nil)
	if err != nil {
		if os.IsPermission(err) {
			return targetUnreadable
		}
		switch {
		case errors.Is(err, ErrSymlink):
			return targetSymlink
		case errors.Is(err, ErrNotRegular):
			return targetNonRegular
		case errors.Is(err, ErrSourceMissing):
			return targetMissing
		default:
			return targetUnreadable
		}
	}
	if newInfo != nil && os.SameFile(current.info, newInfo) && bytes.Equal(current.data, newBytes) {
		return targetNew
	}
	if oldInfo != nil && os.SameFile(current.info, oldInfo) && current.hash == oldHash {
		return targetOld
	}
	return targetOther
}
