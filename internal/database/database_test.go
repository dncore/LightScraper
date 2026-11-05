package database

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

func TestOpenMigratesAndReopenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lightscraper.db")

	store, err := Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	version, err := store.SchemaVersion(context.Background())
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if version != 1 {
		t.Fatalf("expected schema version 1, got %d", version)
	}

	var appliedAt string
	if err := store.db.QueryRow("SELECT applied_at FROM schema_migrations WHERE version = 1").Scan(&appliedAt); err != nil {
		t.Fatalf("read applied_at: %v", err)
	}
	if _, err := time.Parse(time.RFC3339Nano, appliedAt); err != nil {
		t.Fatalf("applied_at is not RFC3339Nano: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("close reopened database: %v", err)
		}
	})

	version, err = reopened.SchemaVersion(context.Background())
	if err != nil {
		t.Fatalf("schema version after reopen: %v", err)
	}
	if version != 1 {
		t.Fatalf("expected schema version 1 after reopen, got %d", version)
	}

	var count int
	if err := reopened.db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one migration row after reopen, got %d", count)
	}

	var appliedAtAfterReopen string
	if err := reopened.db.QueryRow("SELECT applied_at FROM schema_migrations WHERE version = 1").Scan(&appliedAtAfterReopen); err != nil {
		t.Fatalf("read applied_at after reopen: %v", err)
	}
	if appliedAtAfterReopen != appliedAt {
		t.Fatalf("migration was re-applied: before %q, after %q", appliedAt, appliedAtAfterReopen)
	}
}

func TestLoadMigrationsRejectsGaps(t *testing.T) {
	filesystem := fstest.MapFS{
		"migrations/0001_first.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
		"migrations/0003_third.sql": &fstest.MapFile{Data: []byte("SELECT 3;")},
	}
	if _, err := loadMigrations(filesystem); err == nil {
		t.Fatal("expected non-contiguous migrations to fail")
	}
}

func TestValidateAppliedVersions(t *testing.T) {
	tests := []struct {
		name      string
		applied   []int
		latest    int
		wantError bool
	}{
		{name: "empty", applied: nil, latest: 1},
		{name: "contiguous", applied: []int{1, 2, 3}, latest: 3},
		{name: "gap", applied: []int{1, 3}, latest: 3, wantError: true},
		{name: "future", applied: []int{1, 2}, latest: 1, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateAppliedVersions(test.applied, test.latest)
			if test.wantError && err == nil {
				t.Fatal("expected an error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestFailedMigrationRollsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close raw database: %v", err)
		}
	})
	store := &Store{db: db}

	migrations := fstest.MapFS{
		"migrations/0001_broken.sql": &fstest.MapFile{Data: []byte(`
			CREATE TABLE schema_migrations (
				version INTEGER PRIMARY KEY,
				name TEXT NOT NULL,
				applied_at TEXT NOT NULL
			);
			CREATE TABLE should_not_survive (id INTEGER);
			INSERT INTO missing_table (id) VALUES (1);
		`)},
	}
	if err := store.migrate(context.Background(), fs.FS(migrations)); err == nil {
		t.Fatal("expected broken migration to fail")
	}

	exists, err := store.tableExists(context.Background(), "should_not_survive")
	if err != nil {
		t.Fatalf("check rolled-back table: %v", err)
	}
	if exists {
		t.Fatal("table created by failed migration should have been rolled back")
	}
	exists, err = store.tableExists(context.Background(), "schema_migrations")
	if err != nil {
		t.Fatalf("check migration table: %v", err)
	}
	if exists {
		t.Fatal("migration metadata should have been rolled back")
	}
}

func TestOpenRejectsEmptyPath(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Fatal("expected empty path to fail")
	}
}
