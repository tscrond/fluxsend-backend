-- name: MetricsUsersTotal :one
SELECT COUNT(*)::BIGINT AS total_users
FROM users;

-- name: MetricsUsersByPlan :many
SELECT
    COALESCE(p.name, 'unassigned') AS plan_name,
    COUNT(*)::BIGINT AS user_count
FROM users u
LEFT JOIN plans p ON p.id = u.plan_id
GROUP BY COALESCE(p.name, 'unassigned')
ORDER BY plan_name;

-- name: MetricsFilesAndStorageTotals :one
SELECT
    (
        (SELECT COUNT(*)::BIGINT FROM files) +
        (SELECT COUNT(*)::BIGINT FROM workspace_files WHERE file_type != 'inode/directory')
    )::BIGINT AS total_files,
    (
        (SELECT COALESCE(SUM(size), 0)::BIGINT FROM files) +
        (SELECT COALESCE(SUM(size), 0)::BIGINT FROM workspace_files WHERE file_type != 'inode/directory')
    )::BIGINT AS total_storage_bytes;

-- name: MetricsWorkspacesTotal :one
SELECT COUNT(*)::BIGINT AS total_workspaces
FROM workspaces;

-- name: MetricsActiveSharesTotal :one
SELECT COUNT(*)::BIGINT AS active_shares
FROM shares
WHERE expires_at > NOW();

-- name: MetricsActiveAPIKeysByDomain :many
SELECT domain, active_count
FROM (
    SELECT 'private'::TEXT AS domain, COUNT(DISTINCT ak.id)::BIGINT AS active_count
    FROM api_keys ak
    JOIN api_key_user_assignments akua ON akua.api_key_id = ak.id
    WHERE ak.revoked_at IS NULL

    UNION ALL

    SELECT 'workspace'::TEXT AS domain, COUNT(DISTINCT ak.id)::BIGINT AS active_count
    FROM api_keys ak
    JOIN api_key_workspaces akw ON akw.api_key_id = ak.id
    WHERE ak.revoked_at IS NULL
) AS counts
ORDER BY domain;