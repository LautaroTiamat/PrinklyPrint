package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Job struct {
	ID            string
	Filename      string
	Printer       string
	OptionsJSON   string
	MetadataJSON  string
	PDFPath       string
	Status        Status
	Attempts      int
	NextAttemptAt *time.Time
	LastError     string
	SumatraLog    string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	CompletedAt   *time.Time
}

var ErrNotFound = errors.New("job no encontrado")

func (s *Store) RecoverStaleJobs(ctx context.Context) (int, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE jobs
		SET status='queued',
		    last_error=COALESCE(last_error,'') || ' [recuperado tras reinicio del agente]',
		    next_attempt_at=NULL,
		    updated_at=?
		WHERE status='printing'`, time.Now().UTC())
	if err != nil {
		return 0, fmt.Errorf("recover stale jobs: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *Store) CreateJob(ctx context.Context, j Job) error {
	now := time.Now().UTC()
	if j.CreatedAt.IsZero() {
		j.CreatedAt = now
	}
	j.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `INSERT INTO jobs
		(id, filename, printer, options_json, metadata_json, pdf_path, status,
		 attempts, next_attempt_at, last_error, sumatra_log,
		 created_at, updated_at, completed_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		j.ID, j.Filename, j.Printer, j.OptionsJSON, j.MetadataJSON, j.PDFPath,
		j.Status, j.Attempts, j.NextAttemptAt, j.LastError, j.SumatraLog,
		j.CreatedAt, j.UpdatedAt, j.CompletedAt)
	if err != nil {
		return fmt.Errorf("insert job: %w", err)
	}
	return nil
}

func (s *Store) NextDueJob(ctx context.Context, now time.Time) (*Job, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	row := tx.QueryRowContext(ctx, `SELECT id, filename, printer, options_json, metadata_json,
		pdf_path, status, attempts, next_attempt_at, last_error, sumatra_log,
		created_at, updated_at, completed_at
		FROM jobs
		WHERE status='queued' AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
		ORDER BY created_at ASC LIMIT 1`, now)
	j, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE jobs SET status='printing', updated_at=? WHERE id=?`, now, j.ID); err != nil {
		return nil, fmt.Errorf("marcar printing: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	j.Status = StatusPrinting
	j.UpdatedAt = now
	return j, nil
}

func (s *Store) MarkDone(ctx context.Context, id, sumatraLog string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE jobs SET status='done', sumatra_log=?, completed_at=?, updated_at=? WHERE id=?`,
		sumatraLog, now, now, id)
	return err
}

func (s *Store) MarkFailed(ctx context.Context, id string, attempts int, lastErr, sumatraLog string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE jobs SET status='failed', attempts=?, last_error=?, sumatra_log=?, completed_at=?, updated_at=? WHERE id=?`,
		attempts, lastErr, sumatraLog, now, now, id)
	return err
}

func (s *Store) RequeueForRetry(ctx context.Context, id string, attempts int, lastErr, sumatraLog string, nextAttempt time.Time) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE jobs SET status='queued', attempts=?, last_error=?, sumatra_log=?, next_attempt_at=?, updated_at=? WHERE id=?`,
		attempts, lastErr, sumatraLog, nextAttempt, now, id)
	return err
}

func (s *Store) RetryJob(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE jobs SET status='queued', next_attempt_at=NULL, updated_at=? WHERE id=? AND status='failed'`,
		time.Now().UTC(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CancelJob(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE jobs SET status='cancelled', updated_at=? WHERE id=? AND status='queued'`,
		time.Now().UTC(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetJob(ctx context.Context, id string) (*Job, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, filename, printer, options_json, metadata_json,
		pdf_path, status, attempts, next_attempt_at, last_error, sumatra_log,
		created_at, updated_at, completed_at FROM jobs WHERE id=?`, id)
	j, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return j, err
}

type ListJobsFilter struct {
	Status Status
	Limit  int
	Offset int
}

func (s *Store) ListJobs(ctx context.Context, f ListJobsFilter) ([]Job, int, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	var args []any
	where := ""
	if f.Status != "" {
		where = "WHERE status=?"
		args = append(args, string(f.Status))
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := s.db.QueryContext(ctx, `SELECT id, filename, printer, options_json, metadata_json,
		pdf_path, status, attempts, next_attempt_at, last_error, sumatra_log,
		created_at, updated_at, completed_at FROM jobs `+where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *j)
	}
	return out, total, rows.Err()
}

func (s *Store) CountFailedSince(ctx context.Context, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE status='failed' AND updated_at >= ?`, since).Scan(&n)
	return n, err
}

func (s *Store) CountByStatus(ctx context.Context, st Status) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE status=?`, string(st)).Scan(&n)
	return n, err
}

func (s *Store) DeleteOlderThan(ctx context.Context, cutoff time.Time) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT pdf_path FROM jobs WHERE status IN ('done','failed','cancelled') AND COALESCE(completed_at, updated_at) < ?`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		if p != "" {
			paths = append(paths, p)
		}
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM jobs WHERE status IN ('done','failed','cancelled') AND COALESCE(completed_at, updated_at) < ?`, cutoff); err != nil {
		return nil, err
	}
	return paths, nil
}

func (s *Store) PurgeAll(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT pdf_path FROM jobs WHERE status IN ('done','failed','cancelled')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		if p != "" {
			paths = append(paths, p)
		}
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM jobs WHERE status IN ('done','failed','cancelled')`); err != nil {
		return nil, err
	}
	return paths, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanJob(r rowScanner) (*Job, error) {
	var j Job
	var nextAttempt, completed sql.NullTime
	var lastErr, sumatraLog sql.NullString
	err := r.Scan(&j.ID, &j.Filename, &j.Printer, &j.OptionsJSON, &j.MetadataJSON,
		&j.PDFPath, &j.Status, &j.Attempts, &nextAttempt, &lastErr, &sumatraLog,
		&j.CreatedAt, &j.UpdatedAt, &completed)
	if err != nil {
		return nil, err
	}
	if nextAttempt.Valid {
		t := nextAttempt.Time
		j.NextAttemptAt = &t
	}
	if completed.Valid {
		t := completed.Time
		j.CompletedAt = &t
	}
	if lastErr.Valid {
		j.LastError = lastErr.String
	}
	if sumatraLog.Valid {
		j.SumatraLog = sumatraLog.String
	}
	return &j, nil
}
