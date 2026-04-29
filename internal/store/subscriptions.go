package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// AddSubscription marks a series for auto-refresh on every catalog sync. No-op
// if already subscribed.
func (s *Store) AddSubscription(ctx context.Context, seriesID int) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO series_subscriptions(series_id, created_at) VALUES(?, ?)`,
		seriesID, time.Now().Unix())
	return err
}

func (s *Store) RemoveSubscription(ctx context.Context, seriesID int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM series_subscriptions WHERE series_id=?`, seriesID)
	return err
}

func (s *Store) IsSubscribed(ctx context.Context, seriesID int) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM series_subscriptions WHERE series_id=?`, seriesID).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ListSubscriptions returns the series_id of every subscribed show, oldest
// first. Caller is responsible for fetching SeriesRow details if needed.
func (s *Store) ListSubscriptions(ctx context.Context) ([]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT series_id FROM series_subscriptions ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) MarkSubscriptionChecked(ctx context.Context, seriesID int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE series_subscriptions SET last_checked_at=? WHERE series_id=?`,
		time.Now().Unix(), seriesID)
	return err
}

// EpisodeIDsForSeries returns the cached episode IDs for a series across all
// seasons. Used to diff against a fresh get_series_info response to find
// newly-added episodes.
func (s *Store) EpisodeIDsForSeries(ctx context.Context, seriesID int) (map[int]struct{}, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT episode_id FROM episodes WHERE series_id=?`, seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int]struct{})
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}
