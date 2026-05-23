package store

import (
	"context"
	"fmt"
)

var schema = []string{
	`CREATE TABLE IF NOT EXISTS jobs (
		id            TEXT PRIMARY KEY,
		filename      TEXT NOT NULL,
		printer       TEXT NOT NULL,
		options_json  TEXT NOT NULL,
		metadata_json TEXT NOT NULL,
		pdf_path      TEXT NOT NULL,
		status        TEXT NOT NULL,
		attempts      INTEGER NOT NULL DEFAULT 0,
		next_attempt_at DATETIME,
		last_error    TEXT,
		sumatra_log   TEXT,
		created_at    DATETIME NOT NULL,
		updated_at    DATETIME NOT NULL,
		completed_at  DATETIME
	)`,
	`CREATE INDEX IF NOT EXISTS idx_jobs_status  ON jobs(status)`,
	`CREATE INDEX IF NOT EXISTS idx_jobs_created ON jobs(created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_jobs_next_attempt ON jobs(next_attempt_at)`,
	`CREATE TABLE IF NOT EXISTS settings (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
}

func (s *Store) migrate() error {
	ctx := context.Background()
	for _, stmt := range schema {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migración %q: %w", stmt[:40], err)
		}
	}
	return nil
}
