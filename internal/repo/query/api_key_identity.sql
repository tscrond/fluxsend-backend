--- PLACEHOLDER FOR QUERY
-- name: GetIdentityByAPIKey :one
SELECT * FROM identities WHERE user_id = $1;