-- SPDX-License-Identifier: Apache-2.0

-- name: CreateUser :exec
INSERT INTO auth.users (id, email, name, password_hash, disabled, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetUserByEmail :one
SELECT id, email, name, password_hash, disabled, created_at
FROM auth.users
WHERE email = $1;

-- name: ListUsers :many
SELECT id, email, name, disabled, created_at
FROM auth.users
ORDER BY name, id;

-- name: SetUserDisabled :execrows
UPDATE auth.users
SET disabled = $2
WHERE id = $1;

-- name: DeleteUserSessions :exec
DELETE FROM auth.sessions
WHERE user_id = $1;

-- name: CreateSession :exec
INSERT INTO auth.sessions (token_hash, user_id, created_at, expires_at)
VALUES ($1, $2, $3, $4);

-- name: GetUserBySession :one
SELECT u.id, u.email, u.name, u.password_hash, u.disabled, u.created_at
FROM auth.sessions s
JOIN auth.users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.expires_at > $2 AND NOT u.disabled;

-- name: DeleteSession :exec
DELETE FROM auth.sessions
WHERE token_hash = $1;

-- name: DeleteExpiredSessions :execrows
DELETE FROM auth.sessions
WHERE expires_at <= $1;
