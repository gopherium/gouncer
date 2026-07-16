-- SPDX-License-Identifier: Apache-2.0

-- +goose Up
CREATE INDEX sessions_expires_at_idx ON auth.sessions (expires_at);

-- +goose Down
DROP INDEX auth.sessions_expires_at_idx;
