-- +goose Up
-- Initial bootstrap migration. Only meta-level objects so far.

CREATE TABLE IF NOT EXISTS instance_meta (
    key   text PRIMARY KEY,
    value text NOT NULL
);

INSERT INTO instance_meta (key, value)
VALUES ('schema_initialised_at', NOW()::text)
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS instance_meta;
