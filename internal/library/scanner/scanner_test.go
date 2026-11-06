package scanner

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestScanMultipleSystems(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ROM Library")
	mustMkdir(t, root)

	writeTestFile(t, filepath.Join(root, "README.md"), "library notes")
	writeTestFile(t, filepath.Join(root, ".hidden-file"), "hidden")
	writeTestFile(t, filepath.Join(root, "bios", "_info.txt"), infoFor(".nes"))
	writeTestFile(t, filepath.Join(root, "bios", "bios.nes"), "not a game")
	writeTestFile(t, filepath.Join(root, ".hidden-system", "_info.txt"), infoFor(".nes"))
	writeTestFile(t, filepath.Join(root, ".hidden-system", "secret.nes"), "hidden")

	nesDir := filepath.Join(root, "nes")
	writeTestFile(t, filepath.Join(nesDir, "_info.txt"), infoFor(".nes", ".zip", ".txt", ".xml", ".log"))
	writeTestFile(t, filepath.Join(nesDir, "Game (USA).NES"), "rom")
	writeTestFile(t, filepath.Join(nesDir, "sub dir", "Unicode 游戏.zip"), "zip")
	writeTestFile(t, filepath.Join(nesDir, "notes.txt"), "notes")
	writeTestFile(t, filepath.Join(nesDir, "README.txt"), "control")
	writeTestFile(t, filepath.Join(nesDir, "gamelist.xml"), "control")
	writeTestFile(t, filepath.Join(nesDir, "debug.log"), "log")
	writeTestFile(t, filepath.Join(nesDir, ".hidden.nes"), "hidden")
	writeTestFile(t, filepath.Join(nesDir, ".hidden-dir", "secret.nes"), "hidden")

	snesDir := filepath.Join(root, "snes")
	writeTestFile(t, filepath.Join(snesDir, "_info.txt"), infoFor(".sfc", ".zip"))
	writeTestFile(t, filepath.Join(snesDir, "Chrono Trigger.sfc"), "rom")
	writeTestFile(t, filepath.Join(snesDir, "roms.zip"), "zip")

	writeTestFile(t, filepath.Join(root, "empty", "_info.txt"), infoFor(".nes"))
	writeTestFile(t, filepath.Join(root, "noinfo", "orphan.nes"), "orphan")
	writeTestFile(t, filepath.Join(root, "badinfo", "_info.txt"), "not an info file")
	writeTestFile(t, filepath.Join(root, "badinfo", "orphan.nes"), "orphan")

	result, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	expectedIDs := []string{"badinfo", "empty", "nes", "noinfo", "snes"}
	if got := systemIDs(result.Systems); !reflect.DeepEqual(got, expectedIDs) {
		t.Fatalf("expected systems %v, got %v", expectedIDs, got)
	}
	if result.Root != mustAbs(t, root) {
		t.Fatalf("expected absolute root %q, got %q", mustAbs(t, root), result.Root)
	}
	if result.StartedAt.IsZero() || result.FinishedAt.IsZero() || result.FinishedAt.Before(result.StartedAt) {
		t.Fatalf("invalid scan timestamps: start=%v finish=%v", result.StartedAt, result.FinishedAt)
	}
	if result.Stats.Systems != 5 {
		t.Fatalf("expected 5 systems, got %d", result.Stats.Systems)
	}
	if result.Stats.ROMFiles != 5 {
		t.Fatalf("expected 5 ROM files, got %d", result.Stats.ROMFiles)
	}
	if result.Stats.SkippedHidden != 4 {
		t.Fatalf("expected 4 hidden entries skipped, got %d", result.Stats.SkippedHidden)
	}
	if result.Stats.SkippedBios != 1 {
		t.Fatalf("expected bios directory to be skipped, got %d", result.Stats.SkippedBios)
	}
	if result.Stats.Issues != len(result.Issues) || len(result.Issues) != 2 {
		t.Fatalf("expected 2 issues, got stats=%d list=%d", result.Stats.Issues, len(result.Issues))
	}

	nes := requireSystem(t, result, "nes")
	expectedExtensions := []string{".log", ".nes", ".txt", ".xml", ".zip"}
	if !reflect.DeepEqual(nes.Extensions, expectedExtensions) {
		t.Fatalf("expected extensions %v, got %v", expectedExtensions, nes.Extensions)
	}
	if got := romNames(nes.ROMs); !reflect.DeepEqual(got, []string{"Game (USA).NES", "notes.txt", "Unicode 游戏.zip"}) {
		t.Fatalf("unexpected NES ROMs: %v", got)
	}
	if nes.ROMCount != len(nes.ROMs) {
		t.Fatalf("expected ROMCount %d, got %d", len(nes.ROMs), nes.ROMCount)
	}

	nested := nes.ROMs[2]
	if nested.SystemID != "nes" {
		t.Fatalf("unexpected system id: %q", nested.SystemID)
	}
	if nested.RelativePath != "sub dir/Unicode 游戏.zip" {
		t.Fatalf("unexpected relative path: %q", nested.RelativePath)
	}
	if nested.AbsolutePath != filepath.Join(nesDir, "sub dir", "Unicode 游戏.zip") {
		t.Fatalf("unexpected absolute path: %q", nested.AbsolutePath)
	}
	if nested.Extension != ".zip" || nested.DisplayName != "Unicode 游戏" {
		t.Fatalf("unexpected extension/display name: %q / %q", nested.Extension, nested.DisplayName)
	}
	if nested.SizeBytes != int64(len("zip")) || nested.ModTime.IsZero() {
		t.Fatalf("unexpected file metadata: size=%d mtime=%v", nested.SizeBytes, nested.ModTime)
	}
	if nes.ROMs[0].Extension != ".nes" || nes.ROMs[0].DisplayName != "Game (USA)" {
		t.Fatalf("expected case-insensitive extension match, got %+v", nes.ROMs[0])
	}
	if nes.ROMs[1].DisplayName != "notes" {
		t.Fatalf("ordinary .txt file should not be treated as a control file: %+v", nes.ROMs[1])
	}

	snes := requireSystem(t, result, "snes")
	if got := romNames(snes.ROMs); !reflect.DeepEqual(got, []string{"Chrono Trigger.sfc", "roms.zip"}) {
		t.Fatalf("unexpected SNES ROMs: %v", got)
	}

	for _, id := range []string{"noinfo", "badinfo"} {
		system := requireSystem(t, result, id)
		if len(system.ROMs) != 0 {
			t.Fatalf("system %s must not scan with a missing/invalid config: %v", id, system.ROMs)
		}
	}
	if requireSystem(t, result, "noinfo").ROMCount != 0 {
		t.Fatal("system without _info.txt must have zero ROM count")
	}
	if !hasIssueKind(requireSystem(t, result, "noinfo").Issues, IssueInfoMissing) {
		t.Fatal("expected info_missing issue for noinfo")
	}
	if !hasIssueKind(requireSystem(t, result, "badinfo").Issues, IssueInfoInvalid) {
		t.Fatal("expected info_invalid issue for badinfo")
	}
}

func TestScanNoMatchingFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "nes", "_info.txt"), infoFor(".nes"))
	writeTestFile(t, filepath.Join(root, "nes", "readme.zip"), "not a supported extension")

	result, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	system := requireSystem(t, result, "nes")
	if len(system.ROMs) != 0 || system.ROMCount != 0 {
		t.Fatalf("expected no matching ROMs, got %+v", system.ROMs)
	}
	if len(system.Issues) != 0 {
		t.Fatalf("no-match system should not have issues: %v", system.Issues)
	}
	if result.Stats.ROMFiles != 0 {
		t.Fatalf("expected zero ROM files, got %d", result.Stats.ROMFiles)
	}
}

func TestScanRootSymlink(t *testing.T) {
	realRoot := filepath.Join(t.TempDir(), "real-root")
	writeTestFile(t, filepath.Join(realRoot, "nes", "_info.txt"), infoFor(".nes"))
	writeTestFile(t, filepath.Join(realRoot, "nes", "game.nes"), "rom")

	link := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(realRoot, link); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}

	result, err := Scan(context.Background(), link)
	if err != nil {
		t.Fatalf("scan root symlink: %v", err)
	}
	if result.Root != mustAbs(t, link) {
		t.Fatalf("expected logical root %q, got %q", mustAbs(t, link), result.Root)
	}
	system := requireSystem(t, result, "nes")
	if system.Path != filepath.Join(mustAbs(t, link), "nes") {
		t.Fatalf("expected system path under the logical root, got %q", system.Path)
	}
	if len(system.ROMs) != 1 || system.ROMs[0].AbsolutePath != filepath.Join(mustAbs(t, link), "nes", "game.nes") {
		t.Fatalf("unexpected ROM entry through root symlink: %+v", system.ROMs)
	}
}

func TestScanDoesNotFollowDescendantSymlinks(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	systemDir := filepath.Join(root, "nes")
	writeTestFile(t, filepath.Join(systemDir, "_info.txt"), infoFor(".nes"))

	outsideDir := filepath.Join(t.TempDir(), "outside")
	writeTestFile(t, filepath.Join(outsideDir, "outside.nes"), "outside")

	if err := os.Symlink(filepath.Join(outsideDir, "outside.nes"), filepath.Join(systemDir, "linked.nes")); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(systemDir, "linked-dir")); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	if err := os.Symlink(root, filepath.Join(systemDir, "loop")); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(root, "linked-system")); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}

	result, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if result.Stats.SkippedSymlinks < 4 {
		t.Fatalf("expected at least 4 symlinks skipped, got %d", result.Stats.SkippedSymlinks)
	}
	if len(result.Systems) != 1 || result.Systems[0].ID != "nes" {
		t.Fatalf("unexpected systems: %v", systemIDs(result.Systems))
	}
	if len(result.Systems[0].ROMs) != 0 {
		t.Fatalf("symlinked ROMs must not be returned: %v", result.Systems[0].ROMs)
	}
}

func TestInfoSymlinkIsRejected(t *testing.T) {
	outsideInfo := filepath.Join(t.TempDir(), "valid-info.txt")
	writeTestFile(t, outsideInfo, infoFor(".nes"))

	systemDir := filepath.Join(t.TempDir(), "nes")
	mustMkdir(t, systemDir)
	writeTestFile(t, filepath.Join(systemDir, "game.nes"), "rom")
	if err := os.Symlink(outsideInfo, filepath.Join(systemDir, "_info.txt")); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}

	result, err := Scan(context.Background(), filepath.Dir(systemDir))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	system := requireSystem(t, result, "nes")
	if len(system.ROMs) != 0 {
		t.Fatalf("system with symlinked _info.txt must not scan ROMs: %v", system.ROMs)
	}
	if !hasIssueKind(system.Issues, IssueInfoSymlink) {
		t.Fatalf("expected info_symlink issue, got %v", system.Issues)
	}
	if result.Stats.SkippedSymlinks != 1 {
		t.Fatalf("expected one skipped _info.txt symlink, got %d", result.Stats.SkippedSymlinks)
	}
}

func TestInfoFailuresAreRecordedAndOtherSystemsContinue(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"noinfo", "empty", "garbage", "infodir"} {
		mustMkdir(t, filepath.Join(root, id))
		writeTestFile(t, filepath.Join(root, id, "orphan.nes"), "orphan")
	}
	writeTestFile(t, filepath.Join(root, "empty", "_info.txt"), "")
	writeTestFile(t, filepath.Join(root, "garbage", "_info.txt"), "this is not a Batocera info file\n")
	mustMkdir(t, filepath.Join(root, "infodir", "_info.txt"))
	writeTestFile(t, filepath.Join(root, "ok", "_info.txt"), infoFor(".nes"))
	writeTestFile(t, filepath.Join(root, "ok", "game.nes"), "rom")

	result, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(result.Systems) != 5 {
		t.Fatalf("expected all systems to be reported, got %v", systemIDs(result.Systems))
	}
	if requireSystem(t, result, "noinfo").ROMCount != 0 {
		t.Fatal("system without _info.txt must have zero ROM count")
	}
	if !hasIssueKind(requireSystem(t, result, "noinfo").Issues, IssueInfoMissing) {
		t.Fatal("expected info_missing")
	}
	if !hasIssueKind(requireSystem(t, result, "empty").Issues, IssueInfoInvalid) {
		t.Fatal("expected info_invalid for empty file")
	}
	if !hasIssueKind(requireSystem(t, result, "garbage").Issues, IssueInfoInvalid) {
		t.Fatal("expected info_invalid for invalid file")
	}
	if !hasIssueKind(requireSystem(t, result, "infodir").Issues, IssueInfoNotRegular) {
		t.Fatal("expected info_not_regular for directory")
	}
	if len(requireSystem(t, result, "ok").ROMs) != 1 {
		t.Fatal("readable system should still be scanned")
	}
	if result.Stats.Issues != 4 {
		t.Fatalf("expected 4 issues, got %d", result.Stats.Issues)
	}
}

func TestEntryStatFailureIsRecorded(t *testing.T) {
	root := t.TempDir()
	systemDir := filepath.Join(root, "nes")
	writeTestFile(t, filepath.Join(systemDir, "_info.txt"), infoFor(".nes"))
	writeTestFile(t, filepath.Join(systemDir, "ok.nes"), "rom")
	writeTestFile(t, filepath.Join(systemDir, "gone.nes"), "rom")

	result, err := scanWithOps(context.Background(), root, statFailOps{
		target: filepath.Join(systemDir, "gone.nes"),
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	system := requireSystem(t, result, "nes")
	if len(system.ROMs) != 1 || system.ROMs[0].Name != "ok.nes" {
		t.Fatalf("expected only ok.nes, got %v", romNames(system.ROMs))
	}
	issue, ok := issueOfKind(system.Issues, IssueEntryStat)
	if !ok {
		t.Fatalf("expected entry_stat_failed, got %v", system.Issues)
	}
	if issue.SystemID != "nes" || issue.Path != filepath.Join(systemDir, "gone.nes") || issue.Phase != PhaseStat {
		t.Fatalf("issue is not locatable: %+v", issue)
	}
}

func TestInfoReadFailureIsRecorded(t *testing.T) {
	root := t.TempDir()
	infoPath := filepath.Join(root, "nes", "_info.txt")
	writeTestFile(t, infoPath, infoFor(".nes"))
	writeTestFile(t, filepath.Join(root, "nes", "game.nes"), "rom")

	result, err := scanWithOps(context.Background(), root, readFailOps{target: infoPath})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	system := requireSystem(t, result, "nes")
	if len(system.ROMs) != 0 {
		t.Fatalf("expected no ROMs after info read failure: %v", system.ROMs)
	}
	if !hasIssueKind(system.Issues, IssueInfoUnreadable) {
		t.Fatalf("expected info_unreadable, got %v", system.Issues)
	}
}

func TestSystemReadDirFailureIsRecorded(t *testing.T) {
	root := t.TempDir()
	systemDir := filepath.Join(root, "nes")
	writeTestFile(t, filepath.Join(systemDir, "_info.txt"), infoFor(".nes"))
	writeTestFile(t, filepath.Join(systemDir, "game.nes"), "rom")

	result, err := scanWithOps(context.Background(), root, readDirFailOps{target: systemDir})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	system := requireSystem(t, result, "nes")
	if len(system.ROMs) != 0 {
		t.Fatalf("expected no ROMs after directory read failure: %v", system.ROMs)
	}
	if !hasIssueKind(system.Issues, IssueWalk) {
		t.Fatalf("expected walk_error, got %v", system.Issues)
	}
}

func TestScanRejectsInvalidRoots(t *testing.T) {
	fileRoot := filepath.Join(t.TempDir(), "not-a-dir")
	writeTestFile(t, fileRoot, "file")

	tests := []struct {
		name string
		root string
	}{
		{name: "empty", root: ""},
		{name: "missing", root: filepath.Join(t.TempDir(), "missing")},
		{name: "file", root: fileRoot},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := Scan(context.Background(), test.root)
			if err == nil {
				t.Fatal("expected an error")
			}
			if result.FinishedAt.IsZero() {
				t.Fatal("expected FinishedAt to be set on failure")
			}
		})
	}
}

func TestEmptyRoot(t *testing.T) {
	result, err := Scan(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("scan empty root: %v", err)
	}
	if len(result.Systems) != 0 || result.Stats.Systems != 0 || result.Stats.ROMFiles != 0 {
		t.Fatalf("unexpected result for empty root: %+v", result)
	}
}

func TestScanRejectsNilContext(t *testing.T) {
	if _, err := Scan(nil, t.TempDir()); err == nil {
		t.Fatal("expected nil context to be rejected")
	}
}

func TestCanceledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := Scan(ctx, t.TempDir())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if !result.Cancelled {
		t.Fatal("expected Cancelled to be true")
	}
}

func TestCanceledDuringScanReturnsPartialResult(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "a", "_info.txt"), infoFor(".nes"))
	writeTestFile(t, filepath.Join(root, "a", "a.nes"), "rom")
	writeTestFile(t, filepath.Join(root, "b", "_info.txt"), infoFor(".nes"))
	writeTestFile(t, filepath.Join(root, "b", "b.nes"), "rom")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ops := &cancelAfterReadDirOps{at: 2, cancel: cancel}

	result, err := scanWithOps(ctx, root, ops)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if !result.Cancelled {
		t.Fatal("expected Cancelled to be true")
	}
	if len(result.Systems) == 0 {
		t.Fatal("expected a partial system result")
	}
}

func TestScanIsReadOnly(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "nes", "_info.txt"), infoFor(".nes"))
	writeTestFile(t, filepath.Join(root, "nes", "game.nes"), "rom")
	writeTestFile(t, filepath.Join(root, "nes", "nested", "other.nes"), "other")
	writeTestFile(t, filepath.Join(root, "README.md"), "docs")

	before := snapshotTree(t, root)
	if _, err := Scan(context.Background(), root); err != nil {
		t.Fatalf("scan: %v", err)
	}
	after := snapshotTree(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("scan modified the fixture tree\nbefore: %#v\nafter: %#v", before, after)
	}
}

func infoFor(extensions ...string) string {
	parts := make([]string, 0, len(extensions))
	for _, extension := range extensions {
		if !strings.HasPrefix(extension, ".") {
			extension = "." + extension
		}
		parts = append(parts, extension)
	}
	return `ROM files extensions accepted: "` + strings.Join(parts, " ") + "\"\n"
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
}

func mustAbs(t *testing.T, path string) string {
	t.Helper()
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("absolute path for %s: %v", path, err)
	}
	return absolute
}

func systemIDs(systems []SystemResult) []string {
	ids := make([]string, 0, len(systems))
	for _, system := range systems {
		ids = append(ids, system.ID)
	}
	return ids
}

func romNames(roms []ROMEntry) []string {
	names := make([]string, 0, len(roms))
	for _, rom := range roms {
		names = append(names, rom.Name)
	}
	return names
}

func requireSystem(t *testing.T, result ScanResult, id string) SystemResult {
	t.Helper()
	for _, system := range result.Systems {
		if system.ID == id {
			return system
		}
	}
	t.Fatalf("system %q not found in %v", id, systemIDs(result.Systems))
	return SystemResult{}
}

func hasIssueKind(issues []ScanIssue, kind IssueKind) bool {
	_, found := issueOfKind(issues, kind)
	return found
}

func issueOfKind(issues []ScanIssue, kind IssueKind) (ScanIssue, bool) {
	for _, issue := range issues {
		if issue.Kind == kind {
			return issue, true
		}
	}
	return ScanIssue{}, false
}

type statFailOps struct {
	osFileOps
	target string
}

func (o statFailOps) Lstat(name string) (os.FileInfo, error) {
	if name == o.target {
		return nil, fs.ErrNotExist
	}
	return o.osFileOps.Lstat(name)
}

type readFailOps struct {
	osFileOps
	target string
}

func (o readFailOps) ReadFile(name string) ([]byte, error) {
	if name == o.target {
		return nil, fs.ErrPermission
	}
	return o.osFileOps.ReadFile(name)
}

type readDirFailOps struct {
	osFileOps
	target string
}

func (o readDirFailOps) ReadDir(name string) ([]os.DirEntry, error) {
	if name == o.target {
		return nil, fs.ErrPermission
	}
	return o.osFileOps.ReadDir(name)
}

type cancelAfterReadDirOps struct {
	osFileOps
	at     int
	calls  int
	cancel context.CancelFunc
}

func (o *cancelAfterReadDirOps) ReadDir(name string) ([]os.DirEntry, error) {
	entries, err := o.osFileOps.ReadDir(name)
	o.calls++
	if o.calls == o.at {
		o.cancel()
	}
	return entries, err
}

type treeSnapshot struct {
	Mode    os.FileMode
	Size    int64
	ModTime time.Time
	Content string
}

func snapshotTree(t *testing.T, root string) map[string]treeSnapshot {
	t.Helper()
	snapshots := make(map[string]treeSnapshot)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		snapshot := treeSnapshot{
			Mode:    info.Mode(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
		if info.Mode().IsRegular() {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			snapshot.Content = string(content)
		}
		snapshots[filepath.ToSlash(relative)] = snapshot
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot tree: %v", err)
	}
	return snapshots
}
