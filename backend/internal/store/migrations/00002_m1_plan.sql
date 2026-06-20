-- +goose Up
-- +goose StatementBegin

CREATE TABLE athlete_profile (
    id                  INTEGER PRIMARY KEY CHECK (id = 1),
    target_weekly_km    REAL    NOT NULL DEFAULT 20,
    progression_mode    TEXT    NOT NULL DEFAULT 'build',
    zone2_ceiling_bpm   INTEGER,
    threshold_bpm       INTEGER,
    max_hr_bpm          INTEGER,
    run_constraints_json TEXT   NOT NULL DEFAULT '{}',
    goal_text           TEXT    NOT NULL DEFAULT '',
    updated_at          TEXT    NOT NULL
);

CREATE TABLE crossfit_weeks (
    week_start   TEXT PRIMARY KEY,
    image_path   TEXT,
    parsed_json  TEXT NOT NULL,
    raw_response TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE TABLE plans (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    week_start        TEXT    NOT NULL,
    generated_at      TEXT    NOT NULL,
    status            TEXT    NOT NULL DEFAULT 'generated',
    plan_json         TEXT    NOT NULL,
    fitness_summary   TEXT    NOT NULL DEFAULT '',
    context_pack_json TEXT,
    model             TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX idx_plans_week_start ON plans (week_start, generated_at DESC);

INSERT INTO athlete_profile
    (id, target_weekly_km, progression_mode, zone2_ceiling_bpm, threshold_bpm,
     max_hr_bpm, run_constraints_json, goal_text, updated_at)
VALUES
    (1, 20, 'build', NULL, NULL, NULL, '{}', '',
     strftime('%Y-%m-%dT%H:%M:%SZ', 'now'));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE plans;
DROP TABLE crossfit_weeks;
DROP TABLE athlete_profile;
-- +goose StatementEnd
