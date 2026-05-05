BEGIN;
ALTER TABLE plans
	ADD COLUMN max_files_workspace BIGINT NOT NULL DEFAULT 10,
	ADD COLUMN max_user_workspaces BIGINT NOT NULL DEFAULT 1,
	ADD COLUMN max_total_storage_bytes_workspace BIGINT NOT NULL DEFAULT 2147483648,
	ADD COLUMN max_users_workspace BIGINT NOT NULL DEFAULT 2,
	ADD COLUMN max_workspace_folders BIGINT NOT NULL DEFAULT 5;

-- Update existing plans with sensible values
UPDATE plans SET
	max_files_workspace = 10,
	max_user_workspaces = 1,
	max_total_storage_bytes_workspace = 2147483648,
	max_users_workspace = 2,
	max_workspace_folders = 5
WHERE name = 'free';

UPDATE plans SET
	max_files_workspace = 200,
	max_user_workspaces = 10,
	max_total_storage_bytes_workspace = 21474836480,
	max_users_workspace = 10,
	max_workspace_folders = 50
WHERE name = 'developer';

UPDATE plans SET
	max_files_workspace = 2147483647,
	max_user_workspaces = 100,
	max_total_storage_bytes_workspace = 536870912000,
	max_users_workspace = 1000,
	max_workspace_folders = 1000
WHERE name = 'enterprise';

COMMIT;