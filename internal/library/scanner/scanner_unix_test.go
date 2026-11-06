//go:build unix

package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUnreadableRootFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission bits are not enforced for root")
	}

	root := t.TempDir()
	if err := os.Chmod(root, 0); err != nil {
		t.Fatalf("chmod root: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(root, 0o700); err != nil {
			t.Errorf("restore root permissions: %v", err)
		}
	})

	if _, err := Scan(context.Background(), root); err == nil {
		t.Fatal("expected unreadable root to fail")
	}
}

func TestUnreadableInfoIsRecorded(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission bits are not enforced for root")
	}

	root := t.TempDir()
	infoPath := filepath.Join(root, "nes", "_info.txt")
	writeTestFile(t, infoPath, infoFor(".nes"))
	writeTestFile(t, filepath.Join(root, "nes", "game.nes"), "rom")
	if err := os.Chmod(infoPath, 0); err != nil {
		t.Fatalf("chmod info: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(infoPath, 0o644); err != nil {
			t.Errorf("restore info permissions: %v", err)
		}
	})

	result, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	system := requireSystem(t, result, "nes")
	if len(system.ROMs) != 0 {
		t.Fatalf("expected no ROMs after unreadable _info.txt: %v", system.ROMs)
	}
	if !hasIssueKind(system.Issues, IssueInfoUnreadable) {
		t.Fatalf("expected info_unreadable, got %v", system.Issues)
	}
}

func TestUnreadableSystemDirectoryIsRecorded(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission bits are not enforced for root")
	}

	root := t.TempDir()
	systemDir := filepath.Join(root, "nes")
	writeTestFile(t, filepath.Join(systemDir, "_info.txt"), infoFor(".nes"))
	if err := os.Chmod(systemDir, 0); err != nil {
		t.Fatalf("chmod system directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(systemDir, 0o755); err != nil {
			t.Errorf("restore system permissions: %v", err)
		}
	})

	result, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	system := requireSystem(t, result, "nes")
	if !hasIssueKind(system.Issues, IssueInfoUnreadable) {
		t.Fatalf("expected info_unreadable for an unreadable system directory, got %v", system.Issues)
	}
}
