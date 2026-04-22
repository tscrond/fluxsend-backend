-- name: CreateSession :one
INSERT INTO sessions (id, user_id, provider, provider_access_token, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetSession :one
SELECT * FROM sessions WHERE id = $1 AND expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1;
