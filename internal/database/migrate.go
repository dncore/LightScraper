package database

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

var migrationNamePattern = regexp.MustCompile(`^([0-9]{4})_([a-z0-9_]+)\.sql$`)

type migration struct {
	version int
	name    string
	sql     string
}

// migrate is separated from the exported method so tests can exercise the
// runner with an in-memory filesystem.
func (s *Store) migrate(ctx context.Context, migrationsFS fs.FS) error {
	migrations, err := loadMigrations(migrationsFS)
	if err != nil {
		return err
	}

	applied, err := s.appliedVersions(ctx)
	if err != nil {
		return err
	}
	if err := validateAppliedVersions(applied, migrations[len(migrations)-1].version); err != nil {
		return err
	}

	for _, item := range migrations {
		if containsVersion(applied, item.version) {
			continue
		}
		if err := s.applyMigration(ctx, item); err != nil {
			return fmt.Errorf("apply migration %04d_%s: %w", item.version, item.name, err)
		}
	}
	return nil
}

func loadMigrations(migrationsFS fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(migrationsFS, defaultSchemaDir)
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}

	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := migrationNamePattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}

		version, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, fmt.Errorf("parse migration version %q: %w", entry.Name(), err)
		}
		contents, err := fs.ReadFile(migrationsFS, defaultSchemaDir+"/"+entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		if strings.TrimSpace(string(contents)) == "" {
			return nil, fmt.Errorf("migration %q is empty", entry.Name())
		}

		migrations = append(migrations, migration{
			version: version,
			name:    matches[2],
			sql:     string(contents),
		})
	}

	if len(migrations) == 0 {
		return nil, errors.New("no migrations found")
	}
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})
	for index, item := range migrations {
		if item.version != index+1 {
			return nil, fmt.Errorf("migration versions must be contiguous from 1: found %d at position %d", item.version, index+1)
		}
	}
	return migrations, nil
}

func (s *Store) applyMigration(ctx context.Context, item migration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, item.sql); err != nil {
		return fmt.Errorf("execute migration SQL: %w", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		"INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)",
		item.version,
		item.name,
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}

func (s *Store) appliedVersions(ctx context.Context) ([]int, error) {
	exists, err := s.tableExists(ctx, "schema_migrations")
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	rows, err := s.db.QueryContext(ctx, "SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("query schema migrations: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var versions []int
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan schema migration: %w", err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schema migrations: %w", err)
	}
	return versions, nil
}

func (s *Store) tableExists(ctx context.Context, name string) (bool, error) {
	var found string
	err := s.db.QueryRowContext(
		ctx,
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?",
		name,
	).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check table %q: %w", name, err)
	}
	return found == name, nil
}

func validateAppliedVersions(applied []int, latestKnown int) error {
	for index, version := range applied {
		if version != index+1 {
			return fmt.Errorf("migration history is not contiguous: expected version %d, found %d", index+1, version)
		}
	}
	if len(applied) > 0 && applied[len(applied)-1] > latestKnown {
		return fmt.Errorf(
			"database schema version %d is newer than this application supports (%d)",
			applied[len(applied)-1],
			latestKnown,
		)
	}
	return nil
}

func containsVersion(versions []int, target int) bool {
	for _, version := range versions {
		if version == target {
			return true
		}
	}
	return false
}
