-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email, hashed_password)
VALUES (gen_random_uuid(), now(), now(), $1, $2)
RETURNING *;

-- name: DeleteUsers :exec
TRUNCATE users CASCADE;

-- name: GetUserByEmail :one
SELECT id, created_at, updated_at, email, hashed_password
FROM users
WHERE email = $1;

-- name: UpdateEmailAndPassword :one
UPDATE users
SET email = $1, hashed_password = $2, updated_at = now()
WHERE id = $3
RETURNING *;

-- name: UpgradeUserChirpyRed :exec
UPDATE users
SET is_chirpy_red = TRUE
WHERE id = $1;