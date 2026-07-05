-- +goose Up
-- +goose StatementBegin
ALTER TABLE users
  ADD COLUMN notification_preferences jsonb NOT NULL DEFAULT '{}'::jsonb;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE notifications (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  project_id    uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  kind          text NOT NULL,
  task_id       uuid REFERENCES tasks(id) ON DELETE CASCADE,
  actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
  body          text NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  read_at       timestamptz
);
-- +goose StatementEnd

-- +goose StatementBegin
-- Partial index over the unread slice makes the unread-count query cheap
-- (SELECT COUNT(*) WHERE user_id=? AND read_at IS NULL) even as the
-- table grows: PG only scans the rows that satisfy the WHERE clause.
CREATE INDEX notifications_user_unread_idx
  ON notifications (user_id, created_at DESC)
  WHERE read_at IS NULL;
-- +goose StatementEnd

-- +goose StatementBegin
-- Covers the paginated list endpoint (newest-first per user).
CREATE INDEX notifications_user_recent_idx
  ON notifications (user_id, created_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS notifications;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE users DROP COLUMN IF EXISTS notification_preferences;
-- +goose StatementEnd
