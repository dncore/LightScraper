-- LS-T001 bootstrap migration.
-- This migration intentionally creates only migration metadata. Business
-- tables belong to later tasks, after their schemas have been reviewed.

CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at TEXT NOT NULL
);
