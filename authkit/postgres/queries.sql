-- SPDX-License-Identifier: Apache-2.0

-- name: CreateUser :exec
INSERT INTO auth.users (id, email, name, password_hash, disabled, created_at, role, confirmed)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetUserByEmail :one
SELECT id, email, name, password_hash, disabled, created_at, role, confirmed
FROM auth.users
WHERE email = $1;

-- name: GetUserByID :one
SELECT id, email, name, password_hash, disabled, created_at, role, confirmed
FROM auth.users
WHERE id = $1;

-- name: ListUsers :many
SELECT id, email, name, disabled, confirmed, created_at, role
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
SELECT u.id, u.email, u.name, u.password_hash, u.disabled, u.created_at, u.role, u.confirmed
FROM auth.sessions s
JOIN auth.users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.expires_at > $2 AND NOT u.disabled;

-- name: DeleteSession :exec
DELETE FROM auth.sessions
WHERE token_hash = $1;

-- name: DeleteExpiredSessions :execrows
DELETE FROM auth.sessions
WHERE expires_at <= $1;

-- name: SetUserRole :execrows
UPDATE auth.users
SET role = $2
WHERE id = $1;

-- name: GrantRoleToRoleless :execrows
UPDATE auth.users
SET role = $1
WHERE role = '';

-- name: LockEnabledUser :one
SELECT id
FROM auth.users
WHERE id = $1 AND NOT disabled
FOR UPDATE;

-- name: LockUnactivatedUser :one
SELECT id
FROM auth.users
WHERE id = $1 AND NOT disabled AND NOT confirmed
FOR UPDATE;

-- name: CreateToken :exec
INSERT INTO auth.tokens (token_hash, user_id, purpose, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: LiveTokenExists :one
SELECT EXISTS (
    SELECT 1
    FROM auth.tokens
    WHERE user_id = $1 AND purpose = $2 AND expires_at > $3
);

-- name: DeleteUserTokensForPurpose :exec
DELETE FROM auth.tokens
WHERE user_id = $1 AND purpose = $2;

-- name: DeleteUserTokens :exec
DELETE FROM auth.tokens
WHERE user_id = $1;

-- name: FindLiveToken :one
SELECT user_id
FROM auth.tokens
WHERE token_hash = $1 AND purpose = $2 AND expires_at > $3;

-- name: ActivateUser :execrows
UPDATE auth.users
SET password_hash = $2, confirmed = true
WHERE id = $1 AND NOT disabled AND NOT confirmed;

-- name: SetUserPassword :execrows
UPDATE auth.users
SET password_hash = $2
WHERE id = $1 AND NOT disabled;

-- name: DeleteToken :execrows
DELETE FROM auth.tokens
WHERE token_hash = $1;

-- name: ExpiredTokens :many
SELECT user_id, purpose
FROM auth.tokens
WHERE expires_at <= $1;

-- name: DeleteExpiredTokens :execrows
DELETE FROM auth.tokens
WHERE expires_at <= $1;

-- name: DeleteUnconfirmedAccounts :exec
DELETE FROM auth.users
WHERE NOT confirmed
  AND id = ANY($1::uuid[])
  AND NOT EXISTS (
    SELECT 1
    FROM auth.tokens t
    WHERE t.user_id = auth.users.id AND t.purpose = $2 AND t.expires_at > $3
  );
