-- +goose Up
CREATE TABLE IF NOT EXISTS refresh_tokens (
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    token TEXT PRIMARY KEY,
    expires_at TIMESTAMP NOT NULL,
    revoked_at TIMESTAMP,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE refresh_tokens;