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
    p.max_shares_per_day
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
