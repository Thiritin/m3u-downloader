package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

// ErrAlreadyQueued is returned by EnqueueJob when the same (kind, source_id)
// is already in pending or active status. The caller can show a friendly
// "already queued" message instead of a raw SQL constraint error.
var ErrAlreadyQueued = errors.New("already queued")

type JobRow struct {
	ID            int64
	Kind          string
	SourceID      int
	Status        string
	DestPath      string
	ProgressBytes int64
	TotalBytes    int64
	Attempts      int
	LastError     string
	CreatedAt     int64
	StartedAt     *int64
	CompletedAt   *int64
}

func (s *Store) EnqueueJob(ctx context.Context, kind string, sourceID int, destPath string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO jobs(kind,source_id,status,dest_path,created_at)
		VALUES(?,?,?,?,?)`,
		kind, sourceID, "pending", destPath, time.Now().Unix())
	if err != nil {
		// modernc.org/sqlite reports unique-violation as a string match in
		// the error text — there's no typed sentinel we can errors.Is on.
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return 0, ErrAlreadyQueued
		}
		return 0, err
	}
	return res.LastInsertId()
}

// JobStatusBySource returns a map keyed by "kind:source_id" → status for every
// job in the queue. Used by the search/browse views to show queued/done
// badges next to titles.
func (s *Store) JobStatusBySource(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT kind, source_id, status FROM jobs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var kind, status string
		var sourceID int
		if err := rows.Scan(&kind, &sourceID, &status); err != nil {
			return nil, err
		}
		// Keep the most "active" status if the same source has multiple
		// historical job rows (e.g. a failed retry plus a completed run).
		key := JobStatusKey(kind, sourceID)
		if priority(status) > priority(out[key]) {
			out[key] = status
		}
	}
	return out, rows.Err()
}

// JobStatusKey is the canonical map key for JobStatusBySource. Exported so
// the TUI can build keys cheaply for individual lookups.
func JobStatusKey(kind string, sourceID int) string {
	return kind + ":" + strconv.Itoa(sourceID)
}

// priority orders statuses for the "most prominent" rule in JobStatusBySource.
// active > pending > failed > completed > cancelled > "".
func priority(status string) int {
	switch status {
	case "active":
		return 5
	case "pending":
		return 4
	case "failed":
		return 3
	case "completed":
		return 2
	case "cancelled":
		return 1
	}
	return 0
}

func (s *Store) ClaimNext(ctx context.Context) (*JobRow, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	row := tx.QueryRowContext(ctx, `
		SELECT id, kind, source_id, COALESCE(dest_path,''), attempts
		FROM jobs WHERE status='pending'
		ORDER BY created_at ASC LIMIT 1`)
	var j JobRow
	if err := row.Scan(&j.ID, &j.Kind, &j.SourceID, &j.DestPath, &j.Attempts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	now := time.Now().Unix()
	if _, err := tx.ExecContext(ctx,
		`UPDATE jobs SET status='active', started_at=? WHERE id=?`, now, j.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	j.Status = "active"
	j.StartedAt = &now
	return &j, nil
}

func (s *Store) UpdateProgress(ctx context.Context, id int64, progress, total int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET progress_bytes=?, total_bytes=? WHERE id=?`, progress, total, id)
	return err
}

func (s *Store) CompleteJob(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET status='completed', completed_at=? WHERE id=?`, time.Now().Unix(), id)
	return err
}

// RequeueJob marks a job pending again after a transient error,
// clearing started_at, incrementing attempts, recording last_error.
func (s *Store) RequeueJob(ctx context.Context, id int64, lastErr string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET status='pending', attempts=attempts+1, last_error=?, started_at=NULL WHERE id=?`,
		lastErr, id)
	return err
}

// RequeueJobNoAttempt marks a job pending again WITHOUT incrementing
// attempts. Used for provider-side back-pressure (connection limit) where
// we don't want to burn a retry slot on something that's not the job's fault.
func (s *Store) RequeueJobNoAttempt(ctx context.Context, id int64, lastErr string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET status='pending', last_error=?, started_at=NULL WHERE id=?`,
		lastErr, id)
	return err
}

func (s *Store) FailJob(ctx context.Context, id int64, lastErr string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET status='failed', attempts=attempts+1, last_error=?, completed_at=? WHERE id=?`,
		lastErr, time.Now().Unix(), id)
	return err
}

// CancelJob flips an in-flight job to status='cancelled'. The worker's
// per-job poller detects this and aborts the download.
func (s *Store) CancelJob(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET status='cancelled', completed_at=? WHERE id=?`,
		time.Now().Unix(), id)
	return err
}

// DeleteJob removes a job row entirely. Use for pending/failed/completed/
// cancelled rows the user wants out of the queue view.
func (s *Store) DeleteJob(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM jobs WHERE id=?`, id)
	return err
}

// GetJobStatus returns the current status of a job, or "" if it doesn't
// exist (e.g. the user deleted it while it was active).
func (s *Store) GetJobStatus(ctx context.Context, id int64) (string, error) {
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT status FROM jobs WHERE id=?`, id).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return status, err
}

func (s *Store) RetryFailedJob(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET status='pending', attempts=0, last_error=NULL, started_at=NULL, completed_at=NULL WHERE id=?`,
		id)
	return err
}

func (s *Store) ListJobs(ctx context.Context, statuses ...string) ([]JobRow, error) {
	if len(statuses) == 0 {
		statuses = []string{"pending", "active", "completed", "failed", "cancelled"}
	}
	q := `SELECT id,kind,source_id,status,COALESCE(dest_path,''),
	             progress_bytes,COALESCE(total_bytes,0),attempts,COALESCE(last_error,''),
	             created_at,started_at,completed_at
	      FROM jobs WHERE status IN (`
	args := make([]any, 0, len(statuses))
	for i, s := range statuses {
		if i > 0 {
			q += ","
		}
		q += "?"
		args = append(args, s)
	}
	q += `) ORDER BY
	        CASE status WHEN 'active' THEN 0 WHEN 'pending' THEN 1
	                    WHEN 'failed' THEN 2 WHEN 'completed' THEN 3 ELSE 4 END,
	        created_at`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []JobRow
	for rows.Next() {
		var j JobRow
		if err := rows.Scan(&j.ID, &j.Kind, &j.SourceID, &j.Status, &j.DestPath,
			&j.ProgressBytes, &j.TotalBytes, &j.Attempts, &j.LastError,
			&j.CreatedAt, &j.StartedAt, &j.CompletedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
