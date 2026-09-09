-- name: ListUsersAdmin :many
SELECT
    u.id,
    u.user_email,
    u.created_at,
    u.plan_id,
    p.name AS plan_name,
    COUNT(f.id) AS file_count,
    COALESCE(SUM(f.size), 0)::BIGINT AS total_storage_bytes
FROM users u
LEFT JOIN plans p ON p.id = u.plan_id
LEFT JOIN files f ON f.owner_id = u.id
GROUP BY u.id, u.user_email, u.created_at, u.plan_id, p.name
ORDER BY u.created_at DESC;

-- name: GetUserAdminByID :one
SELECT
    u.id,
    u.user_email,
    u.created_at,
    u.plan_id,
    p.name AS plan_name,
    COUNT(f.id) AS file_count,
    COALESCE(SUM(f.size), 0)::BIGINT AS total_storage_bytes
FROM users u
LEFT JOIN plans p ON p.id = u.plan_id
LEFT JOIN files f ON f.owner_id = u.id
WHERE u.id = $1
GROUP BY u.id, u.user_email, u.created_at, u.plan_id, p.name;

-- name: CreatePlanAdmin :one
INSERT INTO plans (
    name,
    max_total_storage_bytes,
    max_file_size_bytes,
    max_files,
    max_files_sent_per_day,
    max_shares_per_day,
    max_files_workspace,
    max_user_workspaces,
    max_total_storage_bytes_workspace,
    max_users_workspace,
    max_workspace_folders,
    max_private_api_keys,
    max_workspace_api_keys
)
VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9,
    $10,
    $11,
    $12,
    $13
)
RETURNING *;

-- name: UpdatePlanAdmin :exec
UPDATE plans
SET
    name = $1,
    max_total_storage_bytes = $2,
    max_file_size_bytes = $3,
    max_files = $4,
    max_files_sent_per_day = $5,
    max_shares_per_day = $6,
    max_files_workspace = $7,
    max_user_workspaces = $8,
    max_total_storage_bytes_workspace = $9,
    max_users_workspace = $10,
    max_workspace_folders = $11,
    max_private_api_keys = $12,
    max_workspace_api_keys = $13
WHERE id = $14;

-- name: ListPlansAdmin :many
SELECT *
FROM plans
ORDER BY created_at DESC;

-- name: ServerCapacitySummary :one
SELECT
    COUNT(u.id)::BIGINT AS total_users,
    COUNT(f.id)::BIGINT AS total_files,
    COALESCE(SUM(f.size), 0)::BIGINT AS total_storage_bytes
FROM users u
LEFT JOIN files f ON f.owner_id = u.id;
