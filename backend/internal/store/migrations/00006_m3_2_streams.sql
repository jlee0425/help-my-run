-- +goose Up
-- +goose StatementBegin
CREATE TABLE activity_streams (
    activity_id INTEGER PRIMARY KEY,
    source      TEXT NOT NULL,
    series_gz   BLOB NOT NULL,
    fetched_at  TEXT NOT NULL,
    FOREIGN KEY (activity_id) REFERENCES activities (strava_id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE stream_analyses (
    activity_id       INTEGER PRIMARY KEY,
    time_in_zone_json TEXT NOT NULL,
    decoupling_pct    REAL,
    pa_hr_first       REAL,
    pa_hr_second      REAL,
    zones_json        TEXT NOT NULL,
    has_hr            INTEGER NOT NULL,
    computed_at       TEXT NOT NULL,
    FOREIGN KEY (activity_id) REFERENCES activities (strava_id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE stream_fetch_log (
    source             TEXT PRIMARY KEY,
    cursor_time        TEXT,
    last_run_at        TEXT,
    last_fetched       INTEGER NOT NULL DEFAULT 0,
    total_fetched      INTEGER NOT NULL DEFAULT 0,
    status             TEXT NOT NULL DEFAULT 'never',
    error              TEXT,
    rate_limited_until TEXT
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO stream_fetch_log (source, status) VALUES ('strava', 'never');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE stream_fetch_log;
DROP TABLE stream_analyses;
DROP TABLE activity_streams;
-- +goose StatementEnd
