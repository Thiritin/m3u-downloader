CREATE TABLE IF NOT EXISTS categories (
  id           INTEGER PRIMARY KEY,
  type         TEXT NOT NULL CHECK(type IN ('vod','series')),
  name         TEXT NOT NULL,
  parent_id    INTEGER,
  fetched_at   INTEGER
);

CREATE TABLE IF NOT EXISTS vods (
  stream_id            INTEGER PRIMARY KEY,
  category_id          INTEGER NOT NULL,
  name                 TEXT NOT NULL,
  year                 INTEGER,
  plot                 TEXT,
  stream_icon_url      TEXT,
  container_extension  TEXT,
  added                INTEGER,
  rating               REAL,
  fetched_at           INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS series (
  series_id    INTEGER PRIMARY KEY,
  category_id  INTEGER NOT NULL,
  name         TEXT NOT NULL,
  year         INTEGER,
  plot         TEXT,
  cover_url    TEXT,
  backdrop_url TEXT,
  fetched_at   INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS seasons (
  series_id      INTEGER NOT NULL,
  season_number  INTEGER NOT NULL,
  name           TEXT,
  overview       TEXT,
  cover_url      TEXT,
  PRIMARY KEY(series_id, season_number)
);

CREATE TABLE IF NOT EXISTS episodes (
  episode_id           INTEGER PRIMARY KEY,
  series_id            INTEGER NOT NULL,
  season_number        INTEGER NOT NULL,
  episode_num          INTEGER NOT NULL,
  title                TEXT,
  plot                 TEXT,
  container_extension  TEXT,
  duration_secs        INTEGER
);

CREATE TABLE IF NOT EXISTS jobs (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  kind            TEXT NOT NULL CHECK(kind IN ('vod','episode')),
  source_id       INTEGER NOT NULL,
  status          TEXT NOT NULL CHECK(status IN
                    ('pending','active','completed','failed','cancelled')),
  dest_path       TEXT,
  progress_bytes  INTEGER NOT NULL DEFAULT 0,
  total_bytes     INTEGER,
  attempts        INTEGER NOT NULL DEFAULT 0,
  last_error      TEXT,
  created_at      INTEGER NOT NULL,
  started_at      INTEGER,
  completed_at    INTEGER
);

CREATE INDEX IF NOT EXISTS idx_jobs_status      ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_categories_type  ON categories(type);
CREATE INDEX IF NOT EXISTS idx_vods_category    ON vods(category_id);
CREATE INDEX IF NOT EXISTS idx_series_category  ON series(category_id);
CREATE INDEX IF NOT EXISTS idx_episodes_series  ON episodes(series_id, season_number);

CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_active_unique
  ON jobs(kind, source_id) WHERE status IN ('pending','active');
