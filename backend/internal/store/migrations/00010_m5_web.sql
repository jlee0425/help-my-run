-- +goose Up
-- M5: web+PWA era. Owner auth (single-row-ish key/value settings + sessions),
-- Web Push subscriptions (replaces Expo device_tokens), profile v2 JSON fields
-- (goals / week rhythm / coach guardrails) captured by the onboarding wizard.
CREATE TABLE app_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE sessions (
    id_hash      TEXT PRIMARY KEY,
    created_at   TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    expires_at   TEXT NOT NULL
);
CREATE TABLE push_subscriptions (
    endpoint   TEXT PRIMARY KEY,
    p256dh     TEXT NOT NULL,
    auth       TEXT NOT NULL,
    created_at TEXT NOT NULL
);
DROP TABLE IF EXISTS device_tokens;
ALTER TABLE athlete_profile ADD COLUMN goals_json      TEXT NOT NULL DEFAULT '[]';
ALTER TABLE athlete_profile ADD COLUMN week_json       TEXT NOT NULL DEFAULT '{}';
ALTER TABLE athlete_profile ADD COLUMN guardrails_json TEXT NOT NULL DEFAULT '{}';

-- +goose Down
-- Note: the three athlete_profile columns are NOT dropped (sqlite DROP COLUMN
-- support predates some deployed DBs; Down is never used in production).
DROP TABLE app_settings;
DROP TABLE sessions;
DROP TABLE push_subscriptions;
CREATE TABLE device_tokens (
    expo_push_token TEXT PRIMARY KEY,
    platform        TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);
