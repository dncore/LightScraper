package gamelist

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func tempFiles(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "gamelist.xml.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func faultDoc(t *testing.T) (string, *Document) {
	t.Helper()
	dir, doc := loadSavedDoc(t, saveTestXML)
	if err := doc.SetField(0, "name", 0, "Changed"); err != nil {
		t.Fatal(err)
	}
	return dir, doc
}

func TestSaveFaultTempCreate(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	ops.createTemp = func(string, string) (*os.File, error) { return nil, errors.New("temp create") }
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrTempCreate) || res.Status != CommitNotCommitted || !res.SnapshotCurrent || doc.Stale() {
		t.Fatalf("result=%#v err=%v stale=%v", res, err, doc.Stale())
	}
	if !bytesEqualFile(t, filepath.Join(dir, "gamelist.xml"), saveTestXML) {
		t.Fatal("target changed")
	}
	if len(backupFiles(t, dir)) != 0 || len(tempFiles(t, dir)) != 0 {
		t.Fatal("temp/backup left behind")
	}
}

func TestSaveFaultTempWrite(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	ops.writeFile = func(*os.File, []byte) error { return errors.New("write") }
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrTempWrite) || res.SnapshotCurrent == false || doc.Stale() {
		t.Fatalf("result=%#v err=%v", res, err)
	}
	if !bytesEqualFile(t, filepath.Join(dir, "gamelist.xml"), saveTestXML) || len(tempFiles(t, dir)) != 0 {
		t.Fatal("target/temp state wrong")
	}
}

func TestSaveFaultTempSync(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	ops.syncFile = func(*os.File) error { return errors.New("sync") }
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrTempSync) || !res.SnapshotCurrent || doc.Stale() {
		t.Fatalf("result=%#v err=%v", res, err)
	}
	if !bytesEqualFile(t, filepath.Join(dir, "gamelist.xml"), saveTestXML) || len(tempFiles(t, dir)) != 0 {
		t.Fatal("target/temp state wrong")
	}
}

func TestSaveFaultTempReadback(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	ops.readFile = func(string) ([]byte, error) { return nil, errors.New("readback") }
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrTempValidate) || !res.SnapshotCurrent || doc.Stale() {
		t.Fatalf("result=%#v err=%v", res, err)
	}
	if !bytesEqualFile(t, filepath.Join(dir, "gamelist.xml"), saveTestXML) || len(tempFiles(t, dir)) != 0 {
		t.Fatal("target/temp state wrong")
	}
}

func TestSaveFaultBackupExists(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	ops.openFile = func(string, int, os.FileMode) (*os.File, error) { return nil, os.ErrExist }
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrBackupExists) || !res.SnapshotCurrent || doc.Stale() {
		t.Fatalf("result=%#v err=%v", res, err)
	}
	if !bytesEqualFile(t, filepath.Join(dir, "gamelist.xml"), saveTestXML) || len(tempFiles(t, dir)) != 0 {
		t.Fatal("target/temp state wrong")
	}
}

func TestSaveFaultBackupOpen(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	ops.openFile = func(string, int, os.FileMode) (*os.File, error) { return nil, errors.New("backup open") }
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrBackup) || !res.SnapshotCurrent || doc.Stale() {
		t.Fatalf("result=%#v err=%v", res, err)
	}
	if !bytesEqualFile(t, filepath.Join(dir, "gamelist.xml"), saveTestXML) || len(tempFiles(t, dir)) != 0 {
		t.Fatal("target/temp state wrong")
	}
}

func TestSaveFaultBackupSync(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	call := 0
	ops.syncFile = func(file *os.File) error {
		call++
		if call == 2 {
			return errors.New("backup sync")
		}
		return file.Sync()
	}
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrBackup) || !res.SnapshotCurrent || doc.Stale() {
		t.Fatalf("result=%#v err=%v", res, err)
	}
	if !bytesEqualFile(t, filepath.Join(dir, "gamelist.xml"), saveTestXML) || len(tempFiles(t, dir)) != 0 {
		t.Fatal("target/temp state wrong")
	}
	if files := backupFiles(t, dir); len(files) != 0 {
		t.Fatalf("partial backup left: %v", files)
	}
}

func TestSaveFaultRenameOld(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	ops.rename = func(string, string) error { return errors.New("rename") }
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrReplace) || res.Status != CommitNotCommitted || !res.SnapshotCurrent || doc.Stale() {
		t.Fatalf("result=%#v err=%v stale=%v", res, err, doc.Stale())
	}
	if !bytesEqualFile(t, filepath.Join(dir, "gamelist.xml"), saveTestXML) || len(backupFiles(t, dir)) != 1 || len(tempFiles(t, dir)) != 0 {
		t.Fatal("unexpected target/backup/temp state")
	}
}

func TestSaveFaultRenameReportedErrorButNew(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	ops.rename = func(oldpath, newpath string) error {
		if err := os.Rename(oldpath, newpath); err != nil {
			return err
		}
		return errors.New("reported failure after rename")
	}
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrReplaceReportedFailure) || res.Status != CommitCommitted || !res.SnapshotCurrent || doc.Stale() {
		t.Fatalf("result=%#v err=%v stale=%v", res, err, doc.Stale())
	}
	if !bytesContains(t, filepath.Join(dir, "gamelist.xml"), `<name>Changed</name>`) {
		t.Fatal("target was not replaced")
	}
	if len(backupFiles(t, dir)) != 1 || len(tempFiles(t, dir)) != 0 {
		t.Fatal("unexpected backup/temp state")
	}
}

func TestSaveFaultRenameSuccessButOtherTarget(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	ops.rename = func(oldpath, newpath string) error {
		if err := os.Rename(oldpath, newpath); err != nil {
			return err
		}
		return os.WriteFile(newpath, []byte("external"), 0o644)
	}
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrPostCommitVerify) || res.Status != CommitCommitted || res.SnapshotCurrent || !doc.Stale() {
		t.Fatalf("result=%#v err=%v stale=%v", res, err, doc.Stale())
	}
	if got := readFile(t, filepath.Join(dir, "gamelist.xml")); !bytes.Equal(got, []byte("external")) {
		t.Fatalf("target = %q", got)
	}
	if len(backupFiles(t, dir)) != 1 || len(tempFiles(t, dir)) != 0 {
		t.Fatal("unexpected backup/temp state")
	}
}

func TestSaveFaultRenameSuccessButMissingTarget(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	ops.rename = func(oldpath, newpath string) error {
		_ = os.Remove(oldpath)
		return os.Remove(newpath)
	}
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrPostCommitVerify) || res.Status != CommitCommitted || res.SnapshotCurrent || !doc.Stale() {
		t.Fatalf("result=%#v err=%v stale=%v", res, err, doc.Stale())
	}
	if _, statErr := os.Lstat(filepath.Join(dir, "gamelist.xml")); !os.IsNotExist(statErr) {
		t.Fatalf("target still exists: %v", statErr)
	}
	if len(backupFiles(t, dir)) != 1 {
		t.Fatal("backup missing")
	}
}

func TestSaveFaultRenameSuccessButOldTarget(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	ops.rename = func(oldpath, newpath string) error {
		return os.Remove(oldpath)
	}
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrCommitUnknown) || res.Status != CommitUnknown || res.SnapshotCurrent || !doc.Stale() {
		t.Fatalf("result=%#v err=%v stale=%v", res, err, doc.Stale())
	}
	if !bytesEqualFile(t, filepath.Join(dir, "gamelist.xml"), saveTestXML) {
		t.Fatal("old target changed")
	}
	if len(backupFiles(t, dir)) != 1 {
		t.Fatal("backup missing")
	}
}

func TestSaveFaultSnapshotRefresh(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	ops.refresh = func(*Document, []byte, os.FileInfo) error { return errors.New("refresh") }
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrSnapshotRefreshFailed) || res.Status != CommitCommitted || res.SnapshotCurrent || !doc.Stale() {
		t.Fatalf("result=%#v err=%v stale=%v", res, err, doc.Stale())
	}
	if !bytesContains(t, filepath.Join(dir, "gamelist.xml"), `<name>Changed</name>`) || len(backupFiles(t, dir)) != 1 {
		t.Fatal("target/backup wrong after committed snapshot failure")
	}
	before := len(backupFiles(t, dir))
	res, err = doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrDocumentStale) || res.SnapshotCurrent || !doc.Stale() {
		t.Fatalf("retry after committed snapshot failure = %#v %v", res, err)
	}
	if len(backupFiles(t, dir)) != before {
		t.Fatal("stale retry created a backup")
	}
}

func TestSaveFaultPostDirSyncWarning(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	call := 0
	ops.syncDir = func(path string) error {
		call++
		if call == 2 {
			return errors.New("post dir sync")
		}
		return nil
	}
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if res.Status != CommitCommitted || !res.SnapshotCurrent || doc.Stale() || len(res.Warnings) == 0 {
		t.Fatalf("result=%#v stale=%v", res, doc.Stale())
	}
	if !errors.Is(res.Warnings[0], ErrDirSyncAfterReplace) {
		t.Fatalf("warning = %v", res.Warnings[0])
	}
	if !bytesContains(t, filepath.Join(dir, "gamelist.xml"), `<name>Changed</name>`) || len(backupFiles(t, dir)) != 1 {
		t.Fatal("target/backup wrong")
	}
}

func TestSaveBestEffortDirSyncUnsupported(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	ops.syncDir = func(string) error { return fmt.Errorf("%w: unsupported", ErrDirSyncUnsupported) }
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{Durability: DurabilityBestEffort})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if res.Status != CommitCommitted || !res.SnapshotCurrent || doc.Stale() || len(res.Warnings) == 0 {
		t.Fatalf("result=%#v stale=%v", res, doc.Stale())
	}
	if !bytesContains(t, filepath.Join(dir, "gamelist.xml"), `<name>Changed</name>`) {
		t.Fatal("target not committed")
	}
}

func TestSaveBestEffortRealDirSyncErrorStops(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	ops.syncDir = func(string) error { return errors.New("EIO") }
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{Durability: DurabilityBestEffort})
	if !errors.Is(err, ErrDirSyncBeforeReplace) || res.Status != CommitNotCommitted || !res.SnapshotCurrent || doc.Stale() {
		t.Fatalf("result=%#v err=%v stale=%v", res, err, doc.Stale())
	}
	if !bytesEqualFile(t, filepath.Join(dir, "gamelist.xml"), saveTestXML) || len(backupFiles(t, dir)) != 1 || len(tempFiles(t, dir)) != 0 {
		t.Fatal("unexpected state")
	}
}

func TestSaveStrictDirSyncUnsupportedStops(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	ops.syncDir = func(string) error { return fmt.Errorf("%w: unsupported", ErrDirSyncUnsupported) }
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{Durability: DurabilityStrict})
	if !errors.Is(err, ErrDirSyncBeforeReplace) || res.Status != CommitNotCommitted || !res.SnapshotCurrent || doc.Stale() {
		t.Fatalf("result=%#v err=%v stale=%v", res, err, doc.Stale())
	}
	if !bytesEqualFile(t, filepath.Join(dir, "gamelist.xml"), saveTestXML) || len(backupFiles(t, dir)) != 1 || len(tempFiles(t, dir)) != 0 {
		t.Fatal("unexpected state")
	}
}

func TestSaveConcurrentChangeAfterBackupMarksStale(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	called := false
	ops.syncDir = func(path string) error {
		if !called {
			called = true
			if err := os.WriteFile(filepath.Join(dir, "gamelist.xml"), []byte(saveTestXML+"<!-- external -->"), 0o644); err != nil {
				return err
			}
		}
		return nil
	}
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{})
	if err == nil || res.Status != CommitNotCommitted || res.SnapshotCurrent || !doc.Stale() {
		t.Fatalf("result=%#v err=%v", res, err)
	}
	if !bytesContains(t, filepath.Join(dir, "gamelist.xml"), "external") {
		t.Fatal("external content was overwritten")
	}
	if len(backupFiles(t, dir)) != 1 || len(tempFiles(t, dir)) != 0 {
		t.Fatal("unexpected backup/temp state")
	}
}

func TestSaveBackupNone(t *testing.T) {
	dir, doc := faultDoc(t)
	res, err := doc.Save(nil, SaveOptions{Backup: BackupNone})
	if err != nil || res.Status != CommitCommitted || !res.SnapshotCurrent || res.BackupPath != "" {
		t.Fatalf("result=%#v err=%v", res, err)
	}
	if got := backupFiles(t, dir); len(got) != 0 {
		t.Fatalf("backup created: %v", got)
	}
}

func bytesEqualFile(t *testing.T, path, want string) bool {
	t.Helper()
	return string(readFile(t, path)) == want
}

func bytesContains(t *testing.T, path, needle string) bool {
	t.Helper()
	return bytes.Contains(readFile(t, path), []byte(needle))
}

func TestSaveMarshalFailure(t *testing.T) {
	dir, doc := faultDoc(t)
	doc.edits = append(doc.edits, edit{kind: 99})
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrSaveMarshal) || res.Status != CommitNotCommitted || !res.SnapshotCurrent || doc.Stale() {
		t.Fatalf("result=%#v err=%v stale=%v", res, err, doc.Stale())
	}
	if !bytesEqualFile(t, filepath.Join(dir, "gamelist.xml"), saveTestXML) || len(backupFiles(t, dir)) != 0 || len(tempFiles(t, dir)) != 0 {
		t.Fatal("unexpected state after Marshal failure")
	}
}

func TestSaveTargetMissingDoesNotCreate(t *testing.T) {
	dir, doc := faultDoc(t)
	target := filepath.Join(dir, "gamelist.xml")
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrSourceMissing) || res.Status != CommitNotCommitted || res.SnapshotCurrent || !doc.Stale() {
		t.Fatalf("result=%#v err=%v stale=%v", res, err, doc.Stale())
	}
	if _, statErr := os.Lstat(target); !os.IsNotExist(statErr) {
		t.Fatalf("Save recreated missing target: %v", statErr)
	}
	if len(backupFiles(t, dir)) != 0 {
		t.Fatal("backup created for missing target")
	}
}

func TestSaveTargetSameBytesDifferentIdentity(t *testing.T) {
	dir, doc := faultDoc(t)
	target := filepath.Join(dir, "gamelist.xml")
	replacement := filepath.Join(dir, "replacement.xml")
	if err := os.WriteFile(replacement, []byte(saveTestXML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, target); err != nil {
		t.Fatal(err)
	}
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrSourceReplaced) || res.Status != CommitNotCommitted || res.SnapshotCurrent || !doc.Stale() {
		t.Fatalf("result=%#v err=%v stale=%v", res, err, doc.Stale())
	}
	if !bytesEqualFile(t, target, saveTestXML) {
		t.Fatal("target was overwritten")
	}
	if len(backupFiles(t, dir)) != 0 {
		t.Fatal("backup created")
	}
}

func TestSaveTargetMetadataChanged(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("chmod semantics differ on Windows")
	}
	dir, doc := faultDoc(t)
	target := filepath.Join(dir, "gamelist.xml")
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrSourceMetadataChanged) || res.Status != CommitNotCommitted || res.SnapshotCurrent || !doc.Stale() {
		t.Fatalf("result=%#v err=%v stale=%v", res, err, doc.Stale())
	}
	if len(backupFiles(t, dir)) != 0 {
		t.Fatal("backup created")
	}
}

func TestSaveRenameSuccessNonRegularStateIsStale(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	ops.rename = func(oldpath, newpath string) error {
		if err := os.Remove(oldpath); err != nil {
			return err
		}
		if err := os.Remove(newpath); err != nil {
			return err
		}
		return os.Mkdir(newpath, 0o755)
	}
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrPostCommitVerify) || res.Status != CommitCommitted || res.SnapshotCurrent || !doc.Stale() {
		t.Fatalf("result=%#v err=%v stale=%v", res, err, doc.Stale())
	}
	if len(backupFiles(t, dir)) != 1 {
		t.Fatal("backup missing")
	}
}

func TestSaveContextCanceledBeforeWrite(t *testing.T) {
	dir, doc := faultDoc(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := doc.Save(ctx, SaveOptions{})
	if !errors.Is(err, context.Canceled) || res.Status != CommitNotCommitted || !res.SnapshotCurrent || doc.Stale() {
		t.Fatalf("result=%#v err=%v stale=%v", res, err, doc.Stale())
	}
	if !bytesEqualFile(t, filepath.Join(dir, "gamelist.xml"), saveTestXML) || len(backupFiles(t, dir)) != 0 || len(tempFiles(t, dir)) != 0 {
		t.Fatal("unexpected state after cancellation")
	}
}

func TestSaveNotCommittedOldTargetCanRetry(t *testing.T) {
	dir, doc := faultDoc(t)
	failing := defaultSaveOperations()
	failing.rename = func(string, string) error { return errors.New("transient rename") }
	doc.ops = failing
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrReplace) || res.Status != CommitNotCommitted || !res.SnapshotCurrent || doc.Stale() {
		t.Fatalf("first Save = %#v %v stale=%v", res, err, doc.Stale())
	}
	if !bytesEqualFile(t, filepath.Join(dir, "gamelist.xml"), saveTestXML) {
		t.Fatal("target changed on failed replace")
	}
	doc.ops = defaultSaveOperations()
	res, err = doc.Save(nil, SaveOptions{})
	if err != nil || res.Status != CommitCommitted || !res.SnapshotCurrent || doc.Stale() {
		t.Fatalf("retry Save = %#v %v stale=%v", res, err, doc.Stale())
	}
	if !bytesContains(t, filepath.Join(dir, "gamelist.xml"), `<name>Changed</name>`) {
		t.Fatal("retry did not write target")
	}
	if len(backupFiles(t, dir)) != 2 {
		t.Fatalf("backups = %v", backupFiles(t, dir))
	}
}

func TestSaveFaultRenameReportedErrorAndRefreshFailure(t *testing.T) {
	_, doc := faultDoc(t)
	ops := defaultSaveOperations()
	ops.rename = func(oldpath, newpath string) error {
		if err := os.Rename(oldpath, newpath); err != nil {
			return err
		}
		return errors.New("reported after replace")
	}
	ops.refresh = func(*Document, []byte, os.FileInfo) error { return errors.New("refresh") }
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrReplaceReportedFailure) || !errors.Is(err, ErrSnapshotRefreshFailed) {
		t.Fatalf("error = %v", err)
	}
	if res.Status != CommitCommitted || res.SnapshotCurrent || !doc.Stale() {
		t.Fatalf("result=%#v stale=%v", res, doc.Stale())
	}
}

func TestSaveFaultBackupWrite(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	call := 0
	ops.writeFile = func(file *os.File, data []byte) error {
		call++
		if call == 2 {
			return errors.New("backup write")
		}
		return writeAll(file, data)
	}
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrBackup) || !res.SnapshotCurrent || doc.Stale() {
		t.Fatalf("result=%#v err=%v", res, err)
	}
	if !bytesEqualFile(t, filepath.Join(dir, "gamelist.xml"), saveTestXML) || len(tempFiles(t, dir)) != 0 {
		t.Fatal("target/temp state wrong")
	}
	if files := backupFiles(t, dir); len(files) != 0 {
		t.Fatalf("partial backup left: %v", files)
	}
}

func TestSaveInvalidOptions(t *testing.T) {
	dir, doc := faultDoc(t)
	res, err := doc.Save(nil, SaveOptions{Backup: BackupPolicy(99)})
	if !errors.Is(err, ErrInvalidSaveOptions) || res.SnapshotCurrent == false || doc.Stale() {
		t.Fatalf("result=%#v err=%v", res, err)
	}
	if !bytesEqualFile(t, filepath.Join(dir, "gamelist.xml"), saveTestXML) || len(backupFiles(t, dir)) != 0 || len(tempFiles(t, dir)) != 0 {
		t.Fatal("invalid options wrote files")
	}
}
