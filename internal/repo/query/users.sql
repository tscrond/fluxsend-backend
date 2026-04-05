-- name: CreateUser :exec
INSERT INTO users (google_id, user_name, user_email, user_bucket)
VALUES ($1, $2, $3, $4)
ON CONFLICT (google_id) DO UPDATE SET user_bucket = EXCLUDED.user_bucket;

-- name: GetUserByGoogleID :one
SELECT * FROM users WHERE google_id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE user_email = $1;

-- name: GetUserBucketById :one
SELECT user_bucket FROM users WHERE google_id = $1;

-- name: UpdateUserBucketNameById :exec
UPDATE users SET user_bucket = $1 WHERE google_id = $2;

-- name: DeleteAccount :one
DELETE FROM users WHERE google_id = $1 RETURNING *;
