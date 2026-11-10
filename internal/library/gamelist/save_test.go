package gamelist

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const saveTestXML = `<gameList>
  <game>
    <path>./Game.nes</path>
    <name>Old</name>
  </game>
</gameList>`

func loadSavedDoc(t *testing.T, data string) (string, *Document) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gamelist.xml"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, doc
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func backupFiles(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "gamelist.xml.bak.*"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestSaveHappyPathAndSnapshotRefresh(t *testing.T) {
	dir, doc := loadSavedDoc(t, saveTestXML)
	oldBytes := []byte(saveTestXML)
	if err := doc.SetField(0, "name", 0, "New"); err != nil {
		t.Fatal(err)
	}
	res, err := doc.Save(nil, SaveOptions{})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if res.Status != CommitCommitted || !res.SnapshotCurrent || !res.Changed || res.BackupPath == "" {
		t.Fatalf("result = %#v", res)
	}
	target := filepath.Join(dir, "gamelist.xml")
	got := readFile(t, target)
	if !bytes.Contains(got, []byte("<name>New</name>")) {
		t.Fatalf("target not updated: %s", got)
	}
	backups := backupFiles(t, dir)
	if len(backups) != 1 {
		t.Fatalf("backups = %v", backups)
	}
	if !bytes.Equal(readFile(t, backups[0]), oldBytes) {
		t.Fatalf("backup does not contain original bytes")
	}
	if doc.Stale() {
		t.Fatalf("Document should be active after successful Save")
	}
	if doc.SourceHash != sha256.Sum256(got) {
		t.Fatalf("SourceHash was not refreshed from target")
	}
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if doc.loadedFileInfo == nil || !os.SameFile(info, doc.loadedFileInfo) {
		t.Fatalf("loadedFileInfo does not identify target")
	}
	if !doc.loadedMtime.Equal(info.ModTime()) || doc.loadedSize != info.Size() || doc.loadedMode != info.Mode() {
		t.Fatalf("snapshot metadata does not match target")
	}
}

func TestSaveNoopDoesNotCreateBackup(t *testing.T) {
	dir, doc := loadSavedDoc(t, saveTestXML)
	before, err := os.Stat(filepath.Join(dir, "gamelist.xml"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := doc.Save(nil, SaveOptions{})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if res.Status != CommitNotCommitted || res.Changed || !res.SnapshotCurrent || doc.Stale() {
		t.Fatalf("no-op result = %#v stale=%v", res, doc.Stale())
	}
	after, err := os.Stat(filepath.Join(dir, "gamelist.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("no-op changed target mtime")
	}
	if got := backupFiles(t, dir); len(got) != 0 {
		t.Fatalf("no-op created backups: %v", got)
	}
}

func TestSaveUsesPrivatePathAfterPublicFieldsChanged(t *testing.T) {
	dir, doc := loadSavedDoc(t, saveTestXML)
	other := t.TempDir()
	doc.Path = filepath.Join(other, "fake.xml")
	doc.SystemDir = other
	doc.SourceHash = [32]byte{1, 2, 3}
	if err := doc.SetField(0, "name", 0, "Private"); err != nil {
		t.Fatal(err)
	}
	res, err := doc.Save(nil, SaveOptions{})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if res.Status != CommitCommitted || !res.SnapshotCurrent {
		t.Fatalf("result = %#v", res)
	}
	if !bytes.Contains(readFile(t, filepath.Join(dir, "gamelist.xml")), []byte("<name>Private</name>")) {
		t.Fatalf("write did not use private loadedPath")
	}
	if _, err := os.Stat(filepath.Join(other, "fake.xml")); !os.IsNotExist(err) {
		t.Fatalf("public Path was used as target")
	}
}

func TestSaveSourceChangedMarksStaleAndNoBackup(t *testing.T) {
	dir, doc := loadSavedDoc(t, saveTestXML)
	target := filepath.Join(dir, "gamelist.xml")
	if err := doc.SetField(0, "name", 0, "New"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(strings.Replace(saveTestXML, "Old", "External", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrSourceChanged) && !errors.Is(err, ErrSourceReplaced) && !errors.Is(err, ErrSourceMetadataChanged) {
		t.Fatalf("Save error = %v", err)
	}
	if res.Status != CommitNotCommitted || res.SnapshotCurrent || !doc.Stale() {
		t.Fatalf("result = %#v stale=%v", res, doc.Stale())
	}
	if got := backupFiles(t, dir); len(got) != 0 {
		t.Fatalf("preflight failure created backups: %v", got)
	}
	if !bytes.Contains(readFile(t, target), []byte("External")) {
		t.Fatalf("external target was overwritten")
	}
}

func TestSaveNoopAfterExternalChangeMarksStale(t *testing.T) {
	dir, doc := loadSavedDoc(t, saveTestXML)
	target := filepath.Join(dir, "gamelist.xml")
	if err := os.WriteFile(target, []byte(strings.Replace(saveTestXML, "Old", "External", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrSourceChanged) && !errors.Is(err, ErrSourceReplaced) && !errors.Is(err, ErrSourceMetadataChanged) {
		t.Fatalf("Save error = %v", err)
	}
	if res.Status != CommitNotCommitted || res.Changed || res.SnapshotCurrent || !doc.Stale() {
		t.Fatalf("result = %#v stale=%v", res, doc.Stale())
	}
}

func TestStaleMethodsAreRejected(t *testing.T) {
	dir, doc := loadSavedDoc(t, saveTestXML)
	target := filepath.Join(dir, "gamelist.xml")
	if err := os.WriteFile(target, []byte(strings.Replace(saveTestXML, "Old", "External", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Save(nil, SaveOptions{}); err == nil {
		t.Fatal("expected stale transition")
	}
	if !doc.Stale() {
		t.Fatal("expected stale Document")
	}
	if err := doc.SetField(0, "name", 0, "X"); !errors.Is(err, ErrDocumentStale) {
		t.Fatalf("SetField = %v", err)
	}
	if err := doc.AddField(0, "developer", "X"); !errors.Is(err, ErrDocumentStale) {
		t.Fatalf("AddField = %v", err)
	}
	if _, err := doc.Marshal(); !errors.Is(err, ErrDocumentStale) {
		t.Fatalf("Marshal = %v", err)
	}
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrDocumentStale) || res.SnapshotCurrent {
		t.Fatalf("Save stale = %#v %v", res, err)
	}
}

func TestSaveConsecutiveUsesActualTargetAsNextBaseline(t *testing.T) {
	dir, doc := loadSavedDoc(t, saveTestXML)
	target := filepath.Join(dir, "gamelist.xml")
	if err := doc.SetField(0, "name", 0, "First"); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Save(nil, SaveOptions{}); err != nil {
		t.Fatal(err)
	}
	first := readFile(t, target)
	if err := doc.SetField(0, "name", 0, "Second"); err != nil {
		t.Fatalf("SetField after Save: %v", err)
	}
	res, err := doc.Save(nil, SaveOptions{})
	if err != nil || res.Status != CommitCommitted || !res.SnapshotCurrent {
		t.Fatalf("second Save = %#v %v", res, err)
	}
	if !bytes.Contains(readFile(t, target), []byte("<name>Second</name>")) {
		t.Fatalf("second target not updated")
	}
	backups := backupFiles(t, dir)
	if len(backups) != 2 {
		t.Fatalf("backups = %v", backups)
	}
	foundFirst := false
	for _, path := range backups {
		if bytes.Equal(readFile(t, path), first) {
			foundFirst = true
		}
	}
	if !foundFirst {
		t.Fatalf("second backup did not snapshot first actual target")
	}
}

func TestSaveParseDocumentHasNoTarget(t *testing.T) {
	doc, err := Parse(t.TempDir(), []byte(saveTestXML))
	if err != nil {
		t.Fatal(err)
	}
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrSaveNoSource) || res.Status != CommitNotCommitted || res.SnapshotCurrent || doc.Stale() {
		t.Fatalf("Parse Save = %#v %v stale=%v", res, err, doc.Stale())
	}
}

func TestLoadRecordsFileAndDirectoryIdentity(t *testing.T) {
	dir, doc := loadSavedDoc(t, saveTestXML)
	target := filepath.Join(dir, "gamelist.xml")
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if doc.loadedFileInfo == nil || !os.SameFile(info, doc.loadedFileInfo) {
		t.Fatalf("loadedFileInfo does not match target")
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if doc.loadedDirInfo == nil || !os.SameFile(dirInfo, doc.loadedDirInfo) {
		t.Fatalf("loadedDirInfo does not match directory")
	}
	if doc.Stale() {
		t.Fatalf("fresh Load is stale")
	}
}

func TestPublicMarshalUsesPublicSystemDir(t *testing.T) {
	_, doc := loadSavedDoc(t, saveTestXML)
	other := t.TempDir()
	doc.SystemDir = other
	if err := doc.SetField(0, "name", 0, "Public"); err != nil {
		t.Fatal(err)
	}
	out, err := doc.Marshal()
	if err != nil {
		t.Fatalf("public Marshal: %v", err)
	}
	if !bytes.Contains(out, []byte("<name>Public</name>")) {
		t.Fatalf("public Marshal output missing edit")
	}
}

func TestReadStableFileRejectsReplacedIdentity(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("removing an open file is not supported on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "gamelist.xml")
	if err := os.WriteFile(path, []byte(saveTestXML), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Keep the original inode allocated so the replacement cannot reuse it.
	hold, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Close()
	replacement := []byte(strings.Replace(saveTestXML, "Old", "Other", 1))
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, replacement, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readStableFile(path, info); !errors.Is(err, ErrSourceReplaced) {
		t.Fatalf("readStableFile = %v, want ErrSourceReplaced", err)
	}
}

func TestSaveRejectsGamelistSymlink(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("symlink permissions differ on Windows")
	}
	dir, doc := loadSavedDoc(t, saveTestXML)
	target := filepath.Join(dir, "gamelist.xml")
	elsewhere := filepath.Join(t.TempDir(), "other.xml")
	if err := os.WriteFile(elsewhere, []byte(saveTestXML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, target); err != nil {
		t.Fatal(err)
	}
	if err := doc.SetField(0, "name", 0, "New"); err != nil {
		t.Fatal(err)
	}
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrSymlink) || res.Status != CommitNotCommitted || res.SnapshotCurrent || !doc.Stale() {
		t.Fatalf("result=%#v err=%v stale=%v", res, err, doc.Stale())
	}
	if got := backupFiles(t, dir); len(got) != 0 {
		t.Fatalf("backup created: %v", got)
	}
}

func TestSaveRejectsChangedSystemDir(t *testing.T) {
	dir, doc := loadSavedDoc(t, saveTestXML)
	if err := doc.SetField(0, "name", 0, "New"); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(t.TempDir(), "moved")
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrSystemDirMissing) || res.Status != CommitNotCommitted || res.SnapshotCurrent || !doc.Stale() {
		t.Fatalf("result=%#v err=%v stale=%v", res, err, doc.Stale())
	}
	if got := backupFiles(t, moved); len(got) != 0 {
		t.Fatalf("backup created: %v", got)
	}
}

func TestSaveAllowsSystemDirSymlink(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("symlink permissions differ on Windows")
	}
	realDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(realDir, "gamelist.xml"), []byte(saveTestXML), 0o644); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(t.TempDir(), "system")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatal(err)
	}
	doc, err := Load(linkDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.SetField(0, "name", 0, "Linked"); err != nil {
		t.Fatal(err)
	}
	res, err := doc.Save(nil, SaveOptions{})
	if err != nil || res.Status != CommitCommitted || !res.SnapshotCurrent {
		t.Fatalf("result=%#v err=%v", res, err)
	}
	if !bytes.Contains(readFile(t, filepath.Join(realDir, "gamelist.xml")), []byte("<name>Linked</name>")) {
		t.Fatal("write through systemDir symlink failed")
	}
}

func TestSaveRenameErrorOtherStateIsUnknown(t *testing.T) {
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	ops.rename = func(oldpath, newpath string) error {
		_ = os.Remove(oldpath)
		_ = os.WriteFile(newpath, []byte("other"), 0o644)
		return errors.New("rename failed after external change")
	}
	doc.ops = ops
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrCommitUnknown) || res.Status != CommitUnknown || res.SnapshotCurrent || !doc.Stale() {
		t.Fatalf("result=%#v err=%v stale=%v", res, err, doc.Stale())
	}
	if !bytes.Equal(readFile(t, filepath.Join(dir, "gamelist.xml")), []byte("other")) {
		t.Fatal("target state was changed")
	}
	if len(backupFiles(t, dir)) != 1 {
		t.Fatal("backup missing")
	}
	before := len(backupFiles(t, dir))
	res, err = doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrDocumentStale) || res.SnapshotCurrent || !doc.Stale() {
		t.Fatalf("stale retry = %#v %v", res, err)
	}
	if len(backupFiles(t, dir)) != before {
		t.Fatal("stale retry created a backup")
	}
}

func TestSaveRenameSuccessSymlinkStateIsStale(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("symlink permissions differ on Windows")
	}
	dir, doc := faultDoc(t)
	ops := defaultSaveOperations()
	ops.rename = func(oldpath, newpath string) error {
		if err := os.Remove(oldpath); err != nil {
			return err
		}
		if err := os.Remove(newpath); err != nil {
			return err
		}
		return os.Symlink(filepath.Join(dir, "missing-target"), newpath)
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

func TestSaveRejectsRetargetedSystemDirSymlink(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("symlink permissions differ on Windows")
	}
	dirA := t.TempDir()
	dirB := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirA, "gamelist.xml"), []byte(saveTestXML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirB, "gamelist.xml"), []byte(saveTestXML), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "system")
	if err := os.Symlink(dirA, link); err != nil {
		t.Fatal(err)
	}
	doc, err := Load(link)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.SetField(0, "name", 0, "New"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dirB, link); err != nil {
		t.Fatal(err)
	}
	res, err := doc.Save(nil, SaveOptions{})
	if !errors.Is(err, ErrSystemDirChanged) || res.Status != CommitNotCommitted || res.SnapshotCurrent || !doc.Stale() {
		t.Fatalf("result=%#v err=%v stale=%v", res, err, doc.Stale())
	}
	if !bytesEqualFile(t, filepath.Join(dirB, "gamelist.xml"), saveTestXML) {
		t.Fatal("retargeted systemDir was written")
	}
}

func TestSavePreservesPermissionBitsForTargetAndBackup(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Chmod permission semantics differ on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "gamelist.xml")
	if err := os.WriteFile(target, []byte(saveTestXML), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o640); err != nil {
		t.Fatal(err)
	}
	doc, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.SetField(0, "name", 0, "Mode"); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Save(nil, SaveOptions{}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("target mode = %o", info.Mode().Perm())
	}
	backups := backupFiles(t, dir)
	if len(backups) != 1 {
		t.Fatalf("backups = %v", backups)
	}
	backupInfo, err := os.Stat(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if backupInfo.Mode().Perm() != 0o640 {
		t.Fatalf("backup mode = %o", backupInfo.Mode().Perm())
	}
}
