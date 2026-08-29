-- SPDX-License-Identifier: Apache-2.0

-- +goose Up
ALTER TABLE auth.users ADD COLUMN confirmed boolean NOT NULL DEFAULT true;

ALTER TABLE auth.users ALTER COLUMN confirmed DROP DEFAULT;

CREATE TABLE auth.tokens (
    token_hash bytea PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES auth.users (id) ON DELETE CASCADE,
    purpose text NOT NULL,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL
);

CREATE INDEX tokens_user_id_purpose_idx ON auth.tokens (user_id, purpose);

CREATE INDEX tokens_expires_at_idx ON auth.tokens (expires_at);

-- +goose Down
DROP TABLE auth.tokens;

ALTER TABLE auth.users DROP COLUMN confirmed;
