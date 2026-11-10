package gamelist

import (
	"os"
)

// saveOperations is a package-internal seam for deterministic fault-injection
// tests. Production code always uses the os-backed defaults.
type saveOperations struct {
	createTemp func(dir, pattern string) (*os.File, error)
	openFile   func(name string, flag int, perm os.FileMode) (*os.File, error)
	readFile   func(name string) ([]byte, error)
	rename     func(oldpath, newpath string) error
	remove     func(name string) error
	syncDir    func(path string) error
	syncFile   func(file *os.File) error
	writeFile  func(file *os.File, data []byte) error
	refresh    func(d *Document, output []byte, tempInfo os.FileInfo) error
}

func defaultSaveOperations() saveOperations {
	return saveOperations{
		createTemp: os.CreateTemp,
		openFile:   os.OpenFile,
		readFile:   os.ReadFile,
		rename:     os.Rename,
		remove:     os.Remove,
		syncDir:    syncDir,
		syncFile: func(file *os.File) error {
			return file.Sync()
		},
		writeFile: writeAll,
		refresh: func(d *Document, output []byte, tempInfo os.FileInfo) error {
			return d.refreshSnapshot(output, tempInfo)
		},
	}
}
