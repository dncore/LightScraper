package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	driverName       = "sqlite"
	openTimeout      = 10 * time.Second
	busyTimeoutMS    = 5000
	maxOpenConns     = 1
	defaultSchemaDir = "migrations"
)

// Store owns the application's SQLite connection. SQLite has a single writer,
// so the skeleton keeps one connection; this also makes the connection-scoped
// PRAGMAs deterministic. A later task can introduce separate read connections
// if measurements justify them.
type Store struct {
	db *sql.DB
	mu sync.Mutex
}

// Open opens path, applies connection PRAGMAs, and runs pending migrations.
// The parent directory must already exist; config.EnsureDataDir owns that
// responsibility so Open remains usable with temporary test directories.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("database path is required")
	}

	db, err := sql.Open(driverName, path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxOpenConns)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)

	ctx, cancel := context.WithTimeout(context.Background(), openTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	pragmas := []string{
		fmt.Sprintf("PRAGMA busy_timeout = %d", busyTimeoutMS),
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
	}
	for _, pragma := range pragmas {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("apply database pragma: %w", err)
		}
	}

	store := &Store{db: db}
	if err := store.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize database schema: %w", err)
	}
	return store, nil
}

// Close releases the database connection. It is safe to call more than once.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	if err != nil {
		return fmt.Errorf("close database: %w", err)
	}
	return nil
}

// Ping verifies that the database is reachable.
func (s *Store) Ping(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return errors.New("database is closed")
	}
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	return nil
}

// SchemaVersion returns the highest applied migration version, or zero when
// the database has not been migrated yet.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return 0, errors.New("database is closed")
	}
	versions, err := s.appliedVersions(ctx)
	if err != nil {
		return 0, err
	}
	if len(versions) == 0 {
		return 0, nil
	}
	return versions[len(versions)-1], nil
}

// Migrate applies all pending embedded migrations. It is idempotent across
// process restarts and safe to call more than once.
func (s *Store) Migrate(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return errors.New("database is closed")
	}
	return s.migrate(ctx, embeddedMigrations)
}
