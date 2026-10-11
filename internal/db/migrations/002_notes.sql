-- The example feature (internal/notes). When you remove it before your first
-- deploy, delete this file too; after a deploy, add a migration that drops it.
CREATE TABLE notes (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX notes_user_id_idx ON notes (user_id, created_at DESC);
