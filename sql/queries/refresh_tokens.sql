-- name: AddRefreshToken :exec
INSERT INTO refresh_tokens (created_at, updated_at, token, expires_at, user_id)
VALUES (now(), now(), $1, $2, $3);

-- name: GetRefreshToken :one
SELECT token, expires_at, revoked_at, user_id
FROM refresh_tokens
WHERE token = $1;

-- name: UpdateRevokedAt :exec
UPDATE refresh_tokens
SET revoked_at = now(), updated_at = now()
WHERE token = $1;