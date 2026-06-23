-- +goose Up
-- +goose StatementBegin
ALTER TABLE activities RENAME COLUMN strava_id TO activity_id;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS strava_tokens;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS oauth_states;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS garmin_activities;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE activities RENAME COLUMN activity_id TO strava_id;
-- +goose StatementEnd
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
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TABLE oauth_states (
    state      TEXT PRIMARY KEY,
    created_at TEXT NOT NULL
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TABLE garmin_activities (
    garmin_activity_id INTEGER PRIMARY KEY,
    start_time         TEXT NOT NULL,
    duration_s         REAL,
    distance_m         REAL,
    activity_type      TEXT,
    raw_json           TEXT NOT NULL
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_garmin_activities_start_time ON garmin_activities (start_time);
-- +goose StatementEnd
