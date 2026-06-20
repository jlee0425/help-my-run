-- +goose Up
-- +goose StatementBegin
CREATE TABLE oauth_states (
    state      TEXT PRIMARY KEY,
    created_at TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE oauth_states;
-- +goose StatementEnd
