-- +goose Up
-- M6: sessions become visible/manageable devices — record where they came from.
ALTER TABLE sessions ADD COLUMN user_agent TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN created_ip TEXT NOT NULL DEFAULT '';

-- +goose Down
-- Columns retained on down (sqlite DROP COLUMN support varies; Down is never
-- used in production).
SELECT 1;
