package gamelist

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Load reads exactly <systemDir>/gamelist.xml using a stable opened-file
// snapshot. It never searches ES fallback locations and never writes. A
// symlink at gamelist.xml is rejected rather than followed.
func Load(systemDir string) (*Document, error) {
	if systemDir == "" {
		return nil, fmt.Errorf("%w: system directory is required", ErrNotFound)
	}
	absolute, err := filepath.Abs(systemDir)
	if err != nil {
		return nil, fmt.Errorf("gamelist: resolve system directory: %w", err)
	}

	dirBefore, err := os.Stat(absolute)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, absolute)
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("%w: %s", ErrUnreadable, absolute)
		}
		return nil, fmt.Errorf("gamelist: stat system directory: %w", err)
	}
	if !dirBefore.IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrSystemDirNotDir, absolute)
	}

	path := filepath.Join(absolute, "gamelist.xml")
	pathBefore, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("%w: %s", ErrUnreadable, path)
		}
		return nil, fmt.Errorf("gamelist: stat %s: %w", path, err)
	}
	if pathBefore.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: %s", ErrSymlink, path)
	}
	if !pathBefore.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s", ErrNotRegular, path)
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrSourceChangedDuringLoad, path)
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("%w: %s", ErrUnreadable, path)
		}
		return nil, fmt.Errorf("gamelist: open %s: %w", path, err)
	}
	defer file.Close()

	openedInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("gamelist: stat opened file: %w", err)
	}
	if !openedInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s", ErrNotRegular, path)
	}

	pathOpened, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrSourceChangedDuringLoad, path, err)
	}
	if pathOpened.Mode()&os.ModeSymlink != 0 || !pathOpened.Mode().IsRegular() || !os.SameFile(pathOpened, openedInfo) {
		return nil, fmt.Errorf("%w: %s: path changed after open", ErrSourceChangedDuringLoad, path)
	}

	data, err := io.ReadAll(file)
	if err != nil {
		if os.IsPermission(err) {
			return nil, fmt.Errorf("%w: %s", ErrUnreadable, path)
		}
		return nil, fmt.Errorf("gamelist: read %s: %w", path, err)
	}
	afterInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("gamelist: stat opened file after read: %w", err)
	}
	if !os.SameFile(afterInfo, openedInfo) ||
		afterInfo.Size() != int64(len(data)) ||
		afterInfo.Mode() != openedInfo.Mode() ||
		!afterInfo.ModTime().Equal(openedInfo.ModTime()) {
		return nil, fmt.Errorf("%w: %s: file changed while reading", ErrSourceChangedDuringLoad, path)
	}

	pathAfter, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrSourceChangedDuringLoad, path, err)
	}
	if pathAfter.Mode()&os.ModeSymlink != 0 || !pathAfter.Mode().IsRegular() || !os.SameFile(pathAfter, afterInfo) {
		return nil, fmt.Errorf("%w: %s: path changed during read", ErrSourceChangedDuringLoad, path)
	}

	dirAfter, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrSystemDirChangedDuringLoad, absolute, err)
	}
	if !dirAfter.IsDir() || !os.SameFile(dirAfter, dirBefore) {
		return nil, fmt.Errorf("%w: %s", ErrSystemDirChangedDuringLoad, absolute)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrEmpty, path)
	}
	doc, err := Parse(absolute, data)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(data)
	doc.Path = path
	doc.loadedPath = path
	doc.loadedDir = absolute
	doc.loadedFileInfo = afterInfo
	doc.loadedHash = hash
	doc.loadedSize = afterInfo.Size()
	doc.loadedMode = afterInfo.Mode()
	doc.loadedMtime = afterInfo.ModTime()
	doc.loadedDirInfo = dirAfter
	doc.SourceHash = hash
	doc.stale = false
	doc.staleReason = nil
	return doc, nil
}
