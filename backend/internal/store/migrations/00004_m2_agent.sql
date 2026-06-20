-- +goose Up
-- +goose StatementBegin

CREATE TABLE device_tokens (
    expo_push_token TEXT PRIMARY KEY,
    platform        TEXT NOT NULL CHECK (platform IN ('ios','android')),
    updated_at      TEXT NOT NULL
);

CREATE TABLE daily_decisions (
    date                  TEXT PRIMARY KEY,
    readiness_color       TEXT NOT NULL CHECK (readiness_color IN ('green','amber','red')),
    drivers_json          TEXT NOT NULL,
    original_session_json TEXT,
    adjusted_session_json TEXT,
    action                TEXT NOT NULL CHECK (action IN ('STAND','SOFTEN','MOVE','REST_DAY')),
    rationale             TEXT NOT NULL DEFAULT '',
    source                TEXT NOT NULL CHECK (source IN ('ai','fallback')),
    raw_response          TEXT,
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL
);

CREATE TABLE agent_runs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    last_run_date TEXT NOT NULL,
    status        TEXT NOT NULL CHECK (status IN ('ok','error')),
    error         TEXT,
    ran_at        TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_agent_runs_last_run_date ON agent_runs (last_run_date);

ALTER TABLE athlete_profile ADD COLUMN daily_run_time TEXT    NOT NULL DEFAULT '05:30';
ALTER TABLE athlete_profile ADD COLUMN timezone       TEXT    NOT NULL DEFAULT 'UTC';
ALTER TABLE athlete_profile ADD COLUMN agent_enabled  INTEGER NOT NULL DEFAULT 1;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE athlete_profile DROP COLUMN agent_enabled;
ALTER TABLE athlete_profile DROP COLUMN timezone;
ALTER TABLE athlete_profile DROP COLUMN daily_run_time;
DROP INDEX idx_agent_runs_last_run_date;
DROP TABLE agent_runs;
DROP TABLE daily_decisions;
DROP TABLE device_tokens;
-- +goose StatementEnd
