-- name: GetIdentityByProvider :one
SELECT * FROM identities WHERE provider = $1 AND provider_user_id = $2;

-- name: CreateIdentity :one
INSERT INTO identities (user_id, provider, provider_user_id, email, email_verified, name, avatar_url)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetIdentityByUserID :one
SELECT * FROM identities WHERE user_id = $1 LIMIT 1;

-- name: GetIdentitiesByUserID :many
SELECT * FROM identities WHERE user_id = $1 ORDER BY created_at ASC;
