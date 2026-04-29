package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// CountAll returns the total number of cached vods and series.
// Used to decide whether the TUI should kick off an initial sync.
func (s *Store) CountAll(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM vods) + (SELECT count(*) FROM series)`).Scan(&n)
	return n, err
}

// ListAllVODs returns every cached VOD (used by global search).
func (s *Store) ListAllVODs(ctx context.Context) ([]VODRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT stream_id,category_id,name,COALESCE(year,0),COALESCE(plot,''),
		        COALESCE(stream_icon_url,''),COALESCE(container_extension,''),
		        COALESCE(added,0),COALESCE(rating,0)
		 FROM vods ORDER BY name`)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []VODRow
	for rows.Next() {
		var v VODRow
		if err := rows.Scan(&v.StreamID, &v.CategoryID, &v.Name, &v.Year, &v.Plot,
			&v.StreamIcon, &v.ContainerExt, &v.Added, &v.Rating); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ListAllSeries returns every cached series (used by global search).
// GetSeries returns the series row for seriesID, or (nil, nil) if not found.
func (s *Store) GetSeries(ctx context.Context, seriesID int) (*SeriesRow, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT series_id,category_id,name,COALESCE(year,0),COALESCE(plot,''),
		        COALESCE(cover_url,''),COALESCE(backdrop_url,'')
		 FROM series WHERE series_id=?`, seriesID)
	var r SeriesRow
	if err := row.Scan(&r.SeriesID, &r.CategoryID, &r.Name, &r.Year, &r.Plot,
		&r.CoverURL, &r.BackdropURL); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

func (s *Store) ListAllSeries(ctx context.Context) ([]SeriesRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT series_id,category_id,name,COALESCE(year,0),COALESCE(plot,''),
		        COALESCE(cover_url,''),COALESCE(backdrop_url,'')
		 FROM series ORDER BY name`)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []SeriesRow
	for rows.Next() {
		var r SeriesRow
		if err := rows.Scan(&r.SeriesID, &r.CategoryID, &r.Name, &r.Year, &r.Plot,
			&r.CoverURL, &r.BackdropURL); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type CategoryRow struct {
	ID        int
	Type      string
	Name      string
	ParentID  int
	FetchedAt int64
}

type VODRow struct {
	StreamID     int
	CategoryID   int
	Name         string
	Year         int
	Plot         string
	StreamIcon   string
	ContainerExt string
	Added        int64
	Rating       float64
}

type SeriesRow struct {
	SeriesID    int
	CategoryID  int
	Name        string
	Year        int
	Plot        string
	CoverURL    string
	BackdropURL string
}

type SeasonRow struct {
	SeriesID     int
	SeasonNumber int
	Name         string
	Overview     string
	CoverURL     string
}

type EpisodeRow struct {
	EpisodeID    int
	SeriesID     int
	SeasonNumber int
	EpisodeNum   int
	Title        string
	Plot         string
	ContainerExt string
	DurationSecs int
}

func (s *Store) UpsertCategories(ctx context.Context, rows []CategoryRow) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO categories(id,type,name,parent_id) VALUES(?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, parent_id=excluded.parent_id, type=excluded.type`)
	if err != nil { return err }
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx, r.ID, r.Type, r.Name, r.ParentID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListCategories(ctx context.Context, kind string) ([]CategoryRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, type, name, COALESCE(parent_id,0), COALESCE(fetched_at,0)
		 FROM categories WHERE type=? ORDER BY name`, kind)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []CategoryRow
	for rows.Next() {
		var c CategoryRow
		if err := rows.Scan(&c.ID, &c.Type, &c.Name, &c.ParentID, &c.FetchedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) MarkCategoryFetched(ctx context.Context, id int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE categories SET fetched_at=? WHERE id=?`,
		time.Now().Unix(), id)
	return err
}

func (s *Store) UpsertVODs(ctx context.Context, rows []VODRow) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO vods(stream_id,category_id,name,year,plot,stream_icon_url,container_extension,added,rating,fetched_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(stream_id) DO UPDATE SET
		  category_id=excluded.category_id, name=excluded.name, year=excluded.year, plot=excluded.plot,
		  stream_icon_url=excluded.stream_icon_url, container_extension=excluded.container_extension,
		  added=excluded.added, rating=excluded.rating, fetched_at=excluded.fetched_at`)
	if err != nil { return err }
	defer stmt.Close()
	now := time.Now().Unix()
	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx, r.StreamID, r.CategoryID, r.Name, r.Year, r.Plot,
			r.StreamIcon, r.ContainerExt, r.Added, r.Rating, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListVODs(ctx context.Context, categoryID int) ([]VODRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT stream_id,category_id,name,COALESCE(year,0),COALESCE(plot,''),
		        COALESCE(stream_icon_url,''),COALESCE(container_extension,''),
		        COALESCE(added,0),COALESCE(rating,0)
		 FROM vods WHERE category_id=? ORDER BY name`, categoryID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []VODRow
	for rows.Next() {
		var v VODRow
		if err := rows.Scan(&v.StreamID, &v.CategoryID, &v.Name, &v.Year, &v.Plot,
			&v.StreamIcon, &v.ContainerExt, &v.Added, &v.Rating); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) UpsertSeries(ctx context.Context, rows []SeriesRow) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO series(series_id,category_id,name,year,plot,cover_url,backdrop_url,fetched_at)
		VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(series_id) DO UPDATE SET
		  category_id=excluded.category_id, name=excluded.name, year=excluded.year, plot=excluded.plot,
		  cover_url=excluded.cover_url, backdrop_url=excluded.backdrop_url, fetched_at=excluded.fetched_at`)
	if err != nil { return err }
	defer stmt.Close()
	now := time.Now().Unix()
	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx, r.SeriesID, r.CategoryID, r.Name, r.Year, r.Plot,
			r.CoverURL, r.BackdropURL, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListSeries(ctx context.Context, categoryID int) ([]SeriesRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT series_id,category_id,name,COALESCE(year,0),COALESCE(plot,''),
		        COALESCE(cover_url,''),COALESCE(backdrop_url,'')
		 FROM series WHERE category_id=? ORDER BY name`, categoryID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []SeriesRow
	for rows.Next() {
		var r SeriesRow
		if err := rows.Scan(&r.SeriesID, &r.CategoryID, &r.Name, &r.Year, &r.Plot,
			&r.CoverURL, &r.BackdropURL); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ReplaceSeasonsAndEpisodes(ctx context.Context, seriesID int, seasons []SeasonRow, episodes []EpisodeRow) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM seasons WHERE series_id=?`, seriesID); err != nil { return err }
	if _, err := tx.ExecContext(ctx, `DELETE FROM episodes WHERE series_id=?`, seriesID); err != nil { return err }
	for _, s := range seasons {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO seasons(series_id,season_number,name,overview,cover_url) VALUES(?,?,?,?,?)`,
			s.SeriesID, s.SeasonNumber, s.Name, s.Overview, s.CoverURL); err != nil {
			return err
		}
	}
	for _, e := range episodes {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO episodes(episode_id,series_id,season_number,episode_num,title,plot,container_extension,duration_secs)
			 VALUES(?,?,?,?,?,?,?,?)`,
			e.EpisodeID, e.SeriesID, e.SeasonNumber, e.EpisodeNum, e.Title, e.Plot, e.ContainerExt, e.DurationSecs); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListSeasons(ctx context.Context, seriesID int) ([]SeasonRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT series_id,season_number,COALESCE(name,''),COALESCE(overview,''),COALESCE(cover_url,'')
		 FROM seasons WHERE series_id=? ORDER BY season_number`, seriesID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []SeasonRow
	for rows.Next() {
		var s SeasonRow
		if err := rows.Scan(&s.SeriesID, &s.SeasonNumber, &s.Name, &s.Overview, &s.CoverURL); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (s *Store) ListEpisodes(ctx context.Context, seriesID, season int) ([]EpisodeRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT episode_id,series_id,season_number,episode_num,COALESCE(title,''),COALESCE(plot,''),
		        COALESCE(container_extension,''),COALESCE(duration_secs,0)
		 FROM episodes WHERE series_id=? AND season_number=? ORDER BY episode_num`, seriesID, season)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []EpisodeRow
	for rows.Next() {
		var e EpisodeRow
		if err := rows.Scan(&e.EpisodeID, &e.SeriesID, &e.SeasonNumber, &e.EpisodeNum, &e.Title,
			&e.Plot, &e.ContainerExt, &e.DurationSecs); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
