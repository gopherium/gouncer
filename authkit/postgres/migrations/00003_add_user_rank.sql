-- SPDX-License-Identifier: Apache-2.0

-- +goose Up
ALTER TABLE auth.users ADD COLUMN rank text NOT NULL DEFAULT '';

CREATE INDEX users_rank_idx ON auth.users (rank) WHERE rank <> '';

-- +goose Down
DROP INDEX auth.users_rank_idx;

ALTER TABLE auth.users DROP COLUMN rank;
