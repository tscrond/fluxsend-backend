-- name: GetUserWithPlan :one
SELECT
    u.id,
    u.user_email,
    u.user_bucket,
    u.created_at,
    p.id AS plan_id,
    p.name AS plan_name,
    p.max_file_size_bytes,
    p.max_total_storage_bytes,
    p.max_files,
    p.max_files_sent_per_day,
    p.max_shares_per_day,
    p.max_user_workspaces,
    p.max_files_workspace,
    p.max_total_storage_bytes_workspace,
    p.max_users_workspace,
    p.max_workspace_folders
FROM users u
JOIN plans p ON u.plan_id = p.id
WHERE u.id = $1;

-- name: GetPlans :many
SELECT * FROM plans;

-- name: GetPlanByName :one
SELECT * FROM plans WHERE name = $1;

-- name: GetPlanFeatures :many
SELECT * FROM plan_features WHERE plan_id = $1;

-- name: UpdateUserPlan :exec
UPDATE users SET plan_id = $1 WHERE id = $2;

-- name: DeleteUserPlan :exec
UPDATE users SET plan_id = NULL WHERE id = $1;

-- name: DeletePlan :exec
DELETE FROM plans WHERE id = $1;

-- name: DeletePlanFeatures :exec
DELETE FROM plan_features WHERE plan_id = $1;

-- name: CheckUploadQuota :one
WITH usage AS (
    SELECT
        COUNT(*)                            AS file_count,
        COALESCE(SUM(size), 0)              AS total_bytes,
        COUNT(*) FILTER (
            WHERE created_at >= NOW() - INTERVAL '1 day'
        )                                   AS files_today
    FROM files
    WHERE owner_id = $1
)
SELECT
    (u.file_count    >= p.max_files)              AS files_exceeded,
    (u.total_bytes   >= p.max_total_storage_bytes) AS storage_exceeded,
    (u.files_today   >= p.max_files_sent_per_day)  AS daily_exceeded
FROM usage u, plans p
WHERE p.id = $2;

-- name: CheckShareQuota :one
SELECT
    (
        SELECT COUNT(*)
        FROM shares
        WHERE shared_by = $1
          AND created_at >= NOW() - INTERVAL '1 day'
    ) >= p.max_shares_per_day AS shares_exceeded
FROM plans p
WHERE p.id = $2;

-- name: GetUserStats :one
WITH file_usage AS (
    SELECT
        COUNT(*)                                                              AS total_files,
        COALESCE(SUM(size), 0)::BIGINT                                        AS total_bytes,
        COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '1 day')       AS files_today,
        COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '7 days')      AS files_last_7d,
        COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '30 days')     AS files_last_30d
    FROM files
    WHERE files.owner_id = $1
),
share_usage AS (
    SELECT
        COUNT(*)                                                              AS total_shares_sent,
        COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '1 day')       AS shares_today,
        COUNT(*) FILTER (WHERE shared_for IS NOT NULL)                        AS targeted_shares,
        COUNT(*) FILTER (WHERE shared_for IS NULL)                            AS public_shares,
        COUNT(*) FILTER (WHERE expires_at > NOW())                            AS active_shares
    FROM shares
    WHERE shares.shared_by = (SELECT user_email FROM users WHERE users.id = $1)
),
received_usage AS (
    SELECT
        COUNT(*) AS total_received
    FROM shares s
    JOIN files f ON f.id = s.file_id
    WHERE s.shared_for = (SELECT user_email FROM users WHERE id = $1)
),
workspace_usage AS (
    SELECT
        COUNT(*) AS owned_workspaces
    FROM workspaces
    WHERE workspaces.owner_id = $1
)
SELECT
    f.total_files,
    f.total_bytes,
    f.files_today,
    f.files_last_7d,
    f.files_last_30d,
    s.total_shares_sent,
    s.shares_today,
    s.targeted_shares,
    s.public_shares,
    s.active_shares,
    r.total_received,
    w.owned_workspaces
FROM file_usage f, share_usage s, received_usage r, workspace_usage w;

-- name: GetUserDailyUploads :many
SELECT
    files.created_at::date AS day,
    COUNT(*)::BIGINT        AS uploads
FROM files
WHERE files.owner_id = $1
  AND files.created_at >= NOW() - INTERVAL '7 days'
GROUP BY files.created_at::date
ORDER BY day;

-- name: GetUserDailyShares :many
SELECT
    shares.created_at::date AS day,
    COUNT(*)::BIGINT         AS shares
FROM shares
WHERE shares.shared_by = (SELECT user_email FROM users WHERE users.id = $1)
  AND shares.created_at >= NOW() - INTERVAL '7 days'
GROUP BY shares.created_at::date
ORDER BY day;
