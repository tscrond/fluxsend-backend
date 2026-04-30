-- name: CreateWorkspace :one
INSERT INTO workspaces
(slug, name, owner_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: CreateWorkspaceMember :one
INSERT INTO workspace_members
(workspace_id, user_id, role)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetWorkspaceBySlug :one
SELECT * FROM workspaces
WHERE slug = $1;

-- name: GetWorkspaceById :one
SELECT id, slug, name, owner_id, created_at FROM workspaces
WHERE id = $1;

-- name: DeleteWorkspace :one
DELETE FROM workspaces
WHERE id = $1
RETURNING *;

-- name: RenameWorkspace :one
UPDATE workspaces
SET name = $2
WHERE id = $1
RETURNING *;

-- name: RenameWorkspaceWithSlug :one
UPDATE workspaces
SET name = $2, slug = $3
WHERE id = $1
RETURNING *;

-- name: GetUserWorkspaces :many
SELECT workspaces.id, workspaces.slug, workspaces.name, workspaces.owner_id, workspaces.created_at, workspace_members.role
FROM workspaces
JOIN workspace_members ON workspaces.id = workspace_members.workspace_id
WHERE workspace_members.user_id = $1;

-- name: GetWorkspaceMembers :many
SELECT u.user_email, u.id, wm.role, wm.joined_at
FROM workspace_members wm
JOIN users u ON u.id = wm.user_id
WHERE wm.workspace_id = $1;

-- name: GetWorkspaceMember :one
SELECT * FROM workspace_members
WHERE workspace_id = $1 AND user_id = $2;

-- name: DeleteWorkspaceMember :exec
DELETE FROM workspace_members
WHERE workspace_id = $1 AND user_id = $2;

-- name: GetWorkspaceInvites :many
SELECT * FROM workspace_invites
WHERE workspace_id = $1;

-- name: CreateWorkspaceInvite :one
INSERT INTO workspace_invites
(workspace_id, email, token, role)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: DeleteWorkspaceInvite :exec
DELETE FROM workspace_invites
WHERE id = $1;

-- name: DeleteWorkspaceInviteByToken :exec
DELETE FROM workspace_invites
WHERE token = $1;

-- name: GetWorkspaceInviteByToken :one
SELECT * FROM workspace_invites
WHERE token = $1;

-- name: GetUserInvitesByEmail :many
SELECT wi.id, wi.workspace_id, wi.email, wi.token, wi.role, wi.expires_at,
       w.name AS workspace_name, w.slug AS workspace_slug
FROM workspace_invites wi
JOIN workspaces w ON w.id = wi.workspace_id
WHERE wi.email = $1 AND wi.expires_at > now();

-- name: GetUserWorkspaceRole :one
SELECT role FROM workspace_members
WHERE workspace_id = $1 AND user_id = $2;

-- name: UpdateWorkspaceMemberRole :exec
UPDATE workspace_members
SET role = $1
WHERE workspace_id = $2 AND user_id = $3;

-- name: CheckWorkspacesPerUserQuota :one
SELECT (
    SELECT COUNT(*)
    FROM workspaces
    WHERE owner_id = $1
) >= p.max_user_workspaces AS workspaces_exceeded
FROM users u
JOIN plans p ON u.plan_id = p.id
WHERE u.id = $1;

-- name: CheckWorkspaceResourceQuota :one
WITH plan AS (
    SELECT
        p.max_files_workspace,
        p.max_total_storage_bytes_workspace,
        p.max_users_workspace,
        p.max_workspace_folders
    FROM workspaces w
    JOIN users u ON u.id = w.owner_id
    JOIN plans p ON u.plan_id = p.id
    WHERE w.id = $1
),
usage AS (
    SELECT
        COUNT(*) FILTER (WHERE file_type != 'inode/directory') AS file_count,
        COALESCE(SUM(size), 0)                                  AS total_bytes,
        COUNT(*) FILTER (WHERE file_type = 'inode/directory')  AS folder_count
    FROM workspace_files
    WHERE workspace_id = $1
),
members AS (
    SELECT COUNT(*) AS member_count
    FROM workspace_members
    WHERE workspace_id = $1
)
SELECT
    (u.file_count   >= pl.max_files_workspace)               AS files_exceeded,
    (u.total_bytes  >= pl.max_total_storage_bytes_workspace) AS storage_exceeded,
    (u.folder_count >= pl.max_workspace_folders)             AS folders_exceeded,
    (m.member_count >= pl.max_users_workspace)               AS users_exceeded
FROM usage u, plan pl, members m;

-- name: GetWorkspaceQuotaDetails :one
WITH plan AS (
    SELECT
        p.max_files_workspace,
        p.max_total_storage_bytes_workspace,
        p.max_users_workspace,
        p.max_workspace_folders
    FROM workspaces w
    JOIN users u ON u.id = w.owner_id
    JOIN plans p ON u.plan_id = p.id
    WHERE w.id = $1
),
usage AS (
    SELECT
        COUNT(*) FILTER (WHERE file_type != 'inode/directory') AS file_count,
        COALESCE(SUM(size), 0)                                  AS total_bytes,
        COUNT(*) FILTER (WHERE file_type = 'inode/directory')  AS folder_count
    FROM workspace_files
    WHERE workspace_id = $1
),
members AS (
    SELECT COUNT(*) AS member_count
    FROM workspace_members
    WHERE workspace_id = $1
)
SELECT
    u.file_count,
    u.total_bytes,
    u.folder_count,
    m.member_count,
    pl.max_files_workspace,
    pl.max_total_storage_bytes_workspace,
    pl.max_users_workspace,
    pl.max_workspace_folders
FROM usage u, plan pl, members m;