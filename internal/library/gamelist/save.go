package gamelist

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Save writes the current Document to its private loaded path. It never uses
// public Path, SystemDir or SourceHash as security credentials, never creates a
// missing gamelist.xml, and never touches ROM or media paths.
func (d *Document) Save(ctx context.Context, opts SaveOptions) (res SaveResult, err error) {
	res = SaveResult{}
	if d == nil {
		return res, ErrSaveNoSource
	}
	res.Path = d.loadedPath
	if d.stale {
		res.Status = CommitNotCommitted
		res.SnapshotCurrent = false
		return res, ErrDocumentStale
	}
	if d.loadedPath == "" || d.loadedDir == "" || d.loadedFileInfo == nil || d.loadedDirInfo == nil {
		res.Status = CommitNotCommitted
		res.SnapshotCurrent = false
		return res, ErrSaveNoSource
	}
	if opts.Backup != BackupDefault && opts.Backup != BackupNone {
		res.Status = CommitNotCommitted
		res.SnapshotCurrent = true
		return res, fmt.Errorf("%w: invalid backup policy", ErrInvalidSaveOptions)
	}
	if opts.Durability != DurabilityBestEffort && opts.Durability != DurabilityStrict {
		res.Status = CommitNotCommitted
		res.SnapshotCurrent = true
		return res, fmt.Errorf("%w: invalid durability policy", ErrInvalidSaveOptions)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	expectedPath := filepath.Join(filepath.Clean(d.loadedDir), "gamelist.xml")
	if filepath.Clean(d.loadedPath) != expectedPath {
		res.Status = CommitNotCommitted
		res.SnapshotCurrent = false
		d.markStale(ErrSaveTarget)
		return res, fmt.Errorf("%w: %s", ErrSaveTarget, d.loadedPath)
	}
	if err := d.checkSystemDir(); err != nil {
		res.Status = CommitNotCommitted
		res.SnapshotCurrent = false
		d.markStale(err)
		return res, err
	}

	current, err := readStableFile(d.loadedPath, d.loadedFileInfo)
	if err != nil {
		res.Status = CommitNotCommitted
		res.SnapshotCurrent = false
		d.markStale(err)
		return res, err
	}
	if err := d.verifyCurrent(current); err != nil {
		res.Status = CommitNotCommitted
		res.SnapshotCurrent = false
		d.markStale(err)
		return res, err
	}
	if err := ctx.Err(); err != nil {
		res.Status = CommitNotCommitted
		res.SnapshotCurrent = true
		return res, err
	}

	output, err := d.marshal(d.loadedDir)
	if err != nil {
		res.Status = CommitNotCommitted
		res.SnapshotCurrent = true
		return res, fmt.Errorf("%w: %w", ErrSaveMarshal, err)
	}
	if bytes.Equal(output, d.source) {
		res.Status = CommitNotCommitted
		res.Changed = false
		res.SnapshotCurrent = true
		return res, nil
	}
	res.Changed = true

	tempPath, tempInfo, err := d.writeTemp(output)
	if err != nil {
		res.Status = CommitNotCommitted
		res.SnapshotCurrent = true
		return res, err
	}
	removeTemp := func() {
		if cleanupErr := removeWith(d.ops.remove, tempPath); cleanupErr != nil {
			res.Warnings = append(res.Warnings, fmt.Errorf("%w: %s: %v", ErrCleanup, tempPath, cleanupErr))
		}
	}

	var backupPath string
	if opts.Backup != BackupNone {
		backupPath, err = createBackup(d.ops, d.loadedDir, d.source, d.loadedMode)
		res.BackupPath = backupPath
		if err != nil {
			removeTemp()
			res.Status = CommitNotCommitted
			res.SnapshotCurrent = true
			return res, err
		}
	}

	if syncErr := d.ops.syncDir(d.loadedDir); syncErr != nil {
		unsupported := errors.Is(syncErr, ErrDirSyncUnsupported) || dirSyncUnsupported(syncErr)
		if unsupported && opts.Durability == DurabilityBestEffort {
			res.Warnings = append(res.Warnings, fmt.Errorf("%w: %v", ErrDirSyncUnsupported, syncErr))
		} else {
			removeTemp()
			res.Status = CommitNotCommitted
			res.SnapshotCurrent = true
			return res, fmt.Errorf("%w: %v", ErrDirSyncBeforeReplace, syncErr)
		}
	}
	if err := ctx.Err(); err != nil {
		removeTemp()
		res.Status = CommitNotCommitted
		res.SnapshotCurrent = true
		return res, err
	}
	if err := d.checkSystemDir(); err != nil {
		removeTemp()
		res.Status = CommitNotCommitted
		res.SnapshotCurrent = false
		d.markStale(err)
		return res, err
	}
	currentAgain, err := readStableFile(d.loadedPath, d.loadedFileInfo)
	if err != nil {
		removeTemp()
		res.Status = CommitNotCommitted
		res.SnapshotCurrent = false
		d.markStale(err)
		return res, err
	}
	if err := d.verifyCurrent(currentAgain); err != nil {
		removeTemp()
		res.Status = CommitNotCommitted
		res.SnapshotCurrent = false
		d.markStale(err)
		return res, err
	}

	renameErr := d.ops.rename(tempPath, d.loadedPath)
	post := inspectTarget(d.loadedPath, d.loadedFileInfo, tempInfo, output, d.loadedHash)

	if syncErr := d.ops.syncDir(d.loadedDir); syncErr != nil {
		res.Warnings = append(res.Warnings, fmt.Errorf("%w: %v", ErrDirSyncAfterReplace, syncErr))
	}
	removeTemp()

	if post == targetNew {
		res.Status = CommitCommitted
		if dirErr := d.checkSystemDir(); dirErr != nil {
			res.SnapshotCurrent = false
			d.markStale(dirErr)
			return res, fmt.Errorf("%w: %w", ErrPostCommitVerify, dirErr)
		}
		if refreshErr := d.ops.refresh(d, output, tempInfo); refreshErr != nil {
			res.SnapshotCurrent = false
			d.markStale(refreshErr)
			if renameErr != nil {
				snapshotErr := fmt.Errorf("%w: %v", ErrSnapshotRefreshFailed, refreshErr)
				return res, fmt.Errorf("%w: %v; %w", ErrReplaceReportedFailure, renameErr, snapshotErr)
			}
			return res, fmt.Errorf("%w: %w", ErrSnapshotRefreshFailed, refreshErr)
		}
		res.SnapshotCurrent = true
		if renameErr != nil {
			return res, fmt.Errorf("%w: %v", ErrReplaceReportedFailure, renameErr)
		}
		return res, nil
	}

	if post == targetOld && renameErr != nil {
		res.Status = CommitNotCommitted
		res.SnapshotCurrent = true
		return res, fmt.Errorf("%w: %v", ErrReplace, renameErr)
	}
	if post == targetOld {
		res.Status = CommitUnknown
		res.SnapshotCurrent = false
		d.markStale(ErrCommitUnknown)
		return res, ErrCommitUnknown
	}

	if renameErr != nil {
		res.Status = CommitUnknown
		res.SnapshotCurrent = false
		d.markStale(ErrCommitUnknown)
		return res, fmt.Errorf("%w: %v", ErrCommitUnknown, renameErr)
	}

	res.Status = CommitCommitted
	res.SnapshotCurrent = false
	d.markStale(ErrPostCommitVerify)
	return res, fmt.Errorf("%w: target state %v", ErrPostCommitVerify, post)
}

func (d *Document) checkSystemDir() error {
	current, err := os.Stat(d.loadedDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrSystemDirMissing, d.loadedDir)
		}
		if os.IsPermission(err) {
			return fmt.Errorf("%w: %s", ErrUnreadable, d.loadedDir)
		}
		return fmt.Errorf("gamelist: stat system directory: %w", err)
	}
	if !current.IsDir() {
		return fmt.Errorf("%w: %s", ErrSystemDirNotDir, d.loadedDir)
	}
	if d.loadedDirInfo == nil || !os.SameFile(current, d.loadedDirInfo) {
		return fmt.Errorf("%w: %s", ErrSystemDirChanged, d.loadedDir)
	}
	return nil
}

func (d *Document) verifyCurrent(current stableRead) error {
	if current.hash != d.loadedHash {
		return fmt.Errorf("%w: %s", ErrSourceChanged, d.loadedPath)
	}
	if current.info.Size() != d.loadedSize ||
		current.info.Mode() != d.loadedMode ||
		!current.info.ModTime().Equal(d.loadedMtime) {
		return fmt.Errorf("%w: %s", ErrSourceMetadataChanged, d.loadedPath)
	}
	return nil
}

func (d *Document) writeTemp(output []byte) (string, os.FileInfo, error) {
	temp, err := d.ops.createTemp(d.loadedDir, "gamelist.xml.tmp-*")
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrTempCreate, err)
	}
	path := temp.Name()
	failed := true
	defer func() {
		if failed {
			_ = temp.Close()
			_ = removeWith(d.ops.remove, path)
		}
	}()

	if err := d.ops.writeFile(temp, output); err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrTempWrite, err)
	}
	if err := temp.Chmod(d.loadedMode.Perm()); err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrTempMetadata, err)
	}
	if err := d.ops.syncFile(temp); err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrTempSync, err)
	}
	info, err := temp.Stat()
	if err != nil {
		return "", nil, fmt.Errorf("%w: stat temp: %v", ErrTempWrite, err)
	}
	if err := temp.Close(); err != nil {
		return "", nil, fmt.Errorf("%w: close temp: %v", ErrTempWrite, err)
	}
	readback, err := d.ops.readFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("%w: readback: %v", ErrTempValidate, err)
	}
	if !bytes.Equal(readback, output) {
		return "", nil, fmt.Errorf("%w: readback bytes differ", ErrTempValidate)
	}
	if _, err := Parse(d.loadedDir, readback); err != nil {
		return "", nil, fmt.Errorf("%w: %w", ErrTempValidate, err)
	}
	failed = false
	return path, info, nil
}

func (d *Document) refreshSnapshot(output []byte, tempInfo os.FileInfo) error {
	current, err := readStableFile(d.loadedPath, tempInfo)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(output)
	if !bytes.Equal(current.data, output) || current.hash != hash {
		return fmt.Errorf("%w: actual target bytes differ from Marshal output", ErrPostCommitVerify)
	}
	parsed, err := Parse(d.loadedDir, current.data)
	if err != nil {
		return err
	}
	d.source = current.data
	d.refs = parsed.refs
	d.Entries = parsed.Entries
	d.Root = parsed.Root
	d.RootAttributes = parsed.RootAttributes
	d.Other = parsed.Other
	d.edits = nil
	d.loadedFileInfo = current.info
	d.loadedHash = current.hash
	d.loadedSize = current.info.Size()
	d.loadedMode = current.info.Mode()
	d.loadedMtime = current.info.ModTime()
	d.SourceHash = current.hash
	d.stale = false
	d.staleReason = nil
	return nil
}

func writeAll(file *os.File, data []byte) error {
	for len(data) > 0 {
		written, err := file.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return fmt.Errorf("short write")
		}
		data = data[written:]
	}
	return nil
}

func createBackup(ops saveOperations, dir string, source []byte, mode os.FileMode) (string, error) {
	path, err := uniqueBackupPath(dir)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrBackup, err)
	}
	file, err := ops.openFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
	if err != nil {
		if os.IsExist(err) {
			return path, fmt.Errorf("%w: %s", ErrBackupExists, path)
		}
		return path, fmt.Errorf("%w: %s: %v", ErrBackup, path, err)
	}
	failed := true
	defer func() {
		if failed {
			_ = file.Close()
			_ = removeWith(ops.remove, path)
		}
	}()
	if err := ops.writeFile(file, source); err != nil {
		return path, fmt.Errorf("%w: %s: %v", ErrBackup, path, err)
	}
	if err := file.Chmod(mode.Perm()); err != nil {
		return path, fmt.Errorf("%w: %s: %v", ErrBackup, path, err)
	}
	if err := ops.syncFile(file); err != nil {
		return path, fmt.Errorf("%w: %s: %v", ErrBackup, path, err)
	}
	if err := file.Close(); err != nil {
		return path, fmt.Errorf("%w: %s: %v", ErrBackup, path, err)
	}
	failed = false
	return path, nil
}

func uniqueBackupPath(dir string) (string, error) {
	var random [6]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return filepath.Join(dir, "gamelist.xml.bak."+time.Now().UTC().Format("20060102T150405.000000000")+"-"+hex.EncodeToString(random[:])), nil
}

func removeWith(remove func(string) error, path string) error {
	if path == "" {
		return nil
	}
	err := remove(path)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}
