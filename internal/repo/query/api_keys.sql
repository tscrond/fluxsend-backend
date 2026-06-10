-- name: CreateAPIKey :one
INSERT INTO api_keys (created_by_user_id, name, key_hash, description, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: CreateAPIKeyScope :exec
INSERT INTO api_key_scopes (api_key_id, scope) VALUES ($1,$2);

-- name: AssignAPIKeyToPrivate :one
INSERT INTO api_key_user_assignments (api_key_id, user_id)
VALUES ($1, $2)
RETURNING *;

-- name: AssignAPIKeyToWorkspace :one
INSERT INTO api_key_workspaces (api_key_id, workspace_id)
VALUES ($1, $2)
RETURNING *;

-- name: ListWorkspaceAPIKeys :many
SELECT
	ak.id,
	ak.created_by_user_id,
	ak.created_at,
	ak.name,
	ak.description,
	ak.status,
	ak.last_used_at
FROM api_keys ak
JOIN api_key_workspaces akw ON akw.api_key_id = ak.id
WHERE akw.workspace_id = $1
	AND ak.revoked_at IS NULL
ORDER BY ak.created_at DESC;

-- name: ListPrivateAPIKeysByUserID :many
SELECT
	ak.id,
	ak.created_by_user_id,
	ak.created_at,
	ak.name,
	ak.description,
	ak.status,
	ak.last_used_at
FROM api_keys ak
JOIN api_key_user_assignments aku ON aku.api_key_id = ak.id
WHERE aku.user_id = $1
	AND ak.revoked_at IS NULL
ORDER BY ak.created_at DESC;

-- name: ListAPIKeyScopes :many
SELECT scope
FROM api_key_scopes
WHERE api_key_id = $1
ORDER BY scope;

-- name: RevokeWorkspaceAPIKey :one
UPDATE api_keys ak
SET status = 'revoked',
	revoked_at = now(),
	revoked_by_user_id = $3
FROM api_key_workspaces akw
WHERE ak.id = $1
	AND akw.api_key_id = ak.id
	AND akw.workspace_id = $2
	AND ak.revoked_at IS NULL
RETURNING ak.*;

-- name: RevokePrivateAPIKey :one
UPDATE api_keys ak
SET status = 'revoked',
	revoked_at = now(),
	revoked_by_user_id = $3
FROM api_key_user_assignments aku
WHERE ak.id = $1
	AND aku.api_key_id = ak.id
	AND aku.user_id = $2
	AND ak.revoked_at IS NULL
RETURNING ak.*;

-- name: CheckPrivateAPIKeyQuota :one
SELECT (
	SELECT COUNT(*)
	FROM api_key_user_assignments a
	JOIN api_keys ak ON ak.id = a.api_key_id
	WHERE a.user_id = $1
		AND ak.revoked_at IS NULL
) >= p.max_private_api_keys AS api_keys_exceeded
FROM users u
JOIN plans p ON u.plan_id = p.id
WHERE u.id = $1;

-- name: GetAPIKey :one
SELECT * FROM api_keys WHERE id = $1;

-- name: GetAuthorizedCLIUserInfoByAPIKey :one
SELECT
		ak.id AS api_key_id,
		COALESCE(akua.user_id, ak.created_by_user_id) AS internal_id,
		u.user_email AS email,
		(
			SELECT i.name
			FROM identities i
			WHERE i.user_id = u.id
			LIMIT 1
		) AS name,
		akua.user_id AS private_user_id,
		akw.workspace_id AS workspace_id
FROM api_keys ak
LEFT JOIN api_key_user_assignments akua ON akua.api_key_id = ak.id
LEFT JOIN api_key_workspaces akw ON akw.api_key_id = ak.id
JOIN users u ON u.id = COALESCE(akua.user_id, ak.created_by_user_id)
WHERE ak.key_hash = crypt($1, ak.key_hash)
	AND ak.revoked_at IS NULL
	AND ak.status = 'active';
