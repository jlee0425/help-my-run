-- +goose Up
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

-- +goose Down
-- +goose StatementBegin
DROP TABLE garmin_activities;
-- +goose StatementEnd
