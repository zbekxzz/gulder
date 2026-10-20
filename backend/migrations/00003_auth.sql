-- M1: учётные данные пользователей (§14: argon2id). Приложение A хранит только email и роль.

-- +goose Up
ALTER TABLE users ADD COLUMN password_hash text;
CREATE INDEX ON audit_events (action, created_at);

-- +goose Down
DROP INDEX audit_events_action_created_at_idx;
ALTER TABLE users DROP COLUMN password_hash;
