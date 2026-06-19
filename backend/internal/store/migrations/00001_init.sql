-- +goose Up
-- +goose StatementBegin

CREATE TABLE strava_tokens (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    access_token  TEXT    NOT NULL,
    refresh_token TEXT    NOT NULL,
    expires_at    INTEGER NOT NULL,
    scope         TEXT,
    athlete_id    INTEGER,
    updated_at    TEXT    NOT NULL
);

CREATE TABLE activities (
    strava_id        INTEGER PRIMARY KEY,
    name             TEXT    NOT NULL,
    type             TEXT    NOT NULL,
    sport_type       TEXT,
    start_time       TEXT    NOT NULL,
    start_time_local TEXT,
    distance_m       REAL    NOT NULL,
    moving_time_s    INTEGER NOT NULL,
    elapsed_time_s   INTEGER NOT NULL,
    avg_hr           REAL,
    max_hr           REAL,
    avg_speed        REAL,
    max_speed        REAL,
    avg_cadence      REAL,
    elevation_gain_m REAL,
    raw_json         TEXT    NOT NULL,
    synced_at        TEXT    NOT NULL
);

CREATE INDEX idx_activities_start_time ON activities (start_time DESC);

CREATE TABLE activity_splits (
    activity_id    INTEGER NOT NULL,
    idx            INTEGER NOT NULL,
    distance_m     REAL    NOT NULL,
    elapsed_time_s INTEGER NOT NULL,
    moving_time_s  INTEGER,
    avg_hr         REAL,
    max_hr         REAL,
    avg_speed      REAL,
    PRIMARY KEY (activity_id, idx),
    FOREIGN KEY (activity_id) REFERENCES activities (strava_id) ON DELETE CASCADE
);

CREATE TABLE garmin_sleep (
    date       TEXT    PRIMARY KEY,
    duration_s INTEGER,
    deep_s     INTEGER,
    light_s    INTEGER,
    rem_s      INTEGER,
    awake_s    INTEGER,
    score      INTEGER,
    raw_json   TEXT    NOT NULL
);

CREATE TABLE garmin_hrv (
    date              TEXT PRIMARY KEY,
    last_night_avg_ms INTEGER,
    status            TEXT,
    raw_json          TEXT NOT NULL
);

CREATE TABLE garmin_body_battery (
    date     TEXT PRIMARY KEY,
    charged  INTEGER,
    drained  INTEGER,
    high     INTEGER,
    low      INTEGER,
    raw_json TEXT NOT NULL
);

CREATE TABLE garmin_rhr (
    date       TEXT    PRIMARY KEY,
    resting_hr INTEGER,
    raw_json   TEXT    NOT NULL
);

CREATE TABLE sync_log (
    source         TEXT PRIMARY KEY,
    last_synced_at TEXT,
    last_run_at    TEXT,
    status         TEXT NOT NULL,
    error          TEXT
);

INSERT INTO sync_log (source, last_synced_at, last_run_at, status, error)
VALUES ('strava', NULL, NULL, 'never', NULL),
       ('garmin', NULL, NULL, 'never', NULL);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE sync_log;
DROP TABLE garmin_rhr;
DROP TABLE garmin_body_battery;
DROP TABLE garmin_hrv;
DROP TABLE garmin_sleep;
DROP TABLE activity_splits;
DROP TABLE activities;
DROP TABLE strava_tokens;
-- +goose StatementEnd
