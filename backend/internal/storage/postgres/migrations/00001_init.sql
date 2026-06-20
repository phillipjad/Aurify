-- +goose Up
-- Initial Aurify schema: users + their DSP connections, and generated covers.
-- The evolving PlaylistAnalysis payload is stored as JSONB on covers; everything
-- else is relational (see docs/adr/0010-postgresql-storage.md).

CREATE TABLE users (
    id           TEXT PRIMARY KEY,
    email        TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);

CREATE TABLE dsp_connections (
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform         TEXT NOT NULL,
    provider_user_id TEXT NOT NULL DEFAULT '',
    access_token     TEXT NOT NULL DEFAULT '',
    refresh_token    TEXT NOT NULL DEFAULT '',
    expires_at       TIMESTAMPTZ NOT NULL,
    scopes           TEXT[] NOT NULL DEFAULT '{}',
    PRIMARY KEY (user_id, platform)
);

CREATE TABLE covers (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform      TEXT NOT NULL,
    playlist_id   TEXT NOT NULL,
    playlist_name TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL,
    prompt        TEXT NOT NULL DEFAULT '',
    image_url     TEXT NOT NULL DEFAULT '',
    analysis      JSONB NOT NULL DEFAULT '{}'::jsonb,
    error         TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_covers_user_created ON covers (user_id, created_at DESC);

-- +goose Down
DROP TABLE covers;
DROP TABLE dsp_connections;
DROP TABLE users;
