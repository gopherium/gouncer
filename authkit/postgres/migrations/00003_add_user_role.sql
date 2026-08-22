-- SPDX-License-Identifier: Apache-2.0

-- +goose Up
ALTER TABLE auth.users ADD COLUMN role text NOT NULL DEFAULT '';

CREATE INDEX users_role_idx ON auth.users (role) WHERE role <> '';

-- +goose Down
DROP INDEX auth.users_role_idx;

ALTER TABLE auth.users DROP COLUMN role;
