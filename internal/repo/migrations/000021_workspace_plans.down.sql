BEGIN;
ALTER TABLE plans
	DROP COLUMN IF EXISTS max_files_workspace,
	DROP COLUMN IF EXISTS max_user_workspaces,
	DROP COLUMN IF EXISTS max_total_storage_bytes_workspace,
	DROP COLUMN IF EXISTS max_users_workspace,
	DROP COLUMN IF EXISTS max_workspace_folders;
COMMIT;
