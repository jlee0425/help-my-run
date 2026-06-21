-- +goose Up
-- +goose StatementBegin
CREATE TABLE garmin_vo2max (
    date     TEXT PRIMARY KEY,
    vo2max   REAL,
    raw_json TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE garmin_vo2max;
-- +goose StatementEnd
