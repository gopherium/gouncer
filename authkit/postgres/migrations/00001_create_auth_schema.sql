-- SPDX-License-Identifier: Apache-2.0

-- +goose Up
CREATE SCHEMA IF NOT EXISTS auth;

CREATE TABLE auth.users (
    id uuid PRIMARY KEY,
    email text NOT NULL UNIQUE,
    name text NOT NULL,
    password_hash text NOT NULL,
    disabled boolean NOT NULL,
    created_at timestamptz NOT NULL
);

CREATE TABLE auth.sessions (
    token_hash bytea PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES auth.users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL
);

CREATE INDEX sessions_user_id_idx ON auth.sessions (user_id);

-- +goose Down
DROP TABLE auth.sessions;
DROP TABLE auth.users;
