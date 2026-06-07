BEGIN;

CREATE TABLE IF NOT EXISTS api_keys (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
	name TEXT NOT NULL,
	key_hash TEXT NOT NULL UNIQUE,
	description TEXT,
	status TEXT NOT NULL DEFAULT 'active' CHECK (
			status IN ('active', 'disabled', 'revoked')
	),
	last_used_at TIMESTAMP WITH TIME ZONE,
	revoked_at TIMESTAMP WITH TIME ZONE,
	revoked_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_api_keys_created_by_user_id
	ON api_keys(created_by_user_id);

CREATE INDEX IF NOT EXISTS idx_api_keys_status
	ON api_keys(status);

CREATE TABLE IF NOT EXISTS api_key_user_assignments (
	api_key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	assigned_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
	PRIMARY KEY (api_key_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_api_key_user_assignments_user_id
	ON api_key_user_assignments(user_id);

CREATE TABLE IF NOT EXISTS api_key_scopes (
	api_key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
	scope TEXT NOT NULL CHECK (
		scope IN (
			'private_files:read',
			'private_files:write',
			'private_files:delete',
			'private_files:share',
			'workspaces:read',
			'workspaces:write',
			'workspaces:delete',
			'workspaces:members:read',
			'workspaces:members:manage',
			'workspaces:invites:manage',
			'workspaces:files:read',
			'workspaces:files:write',
			'workspaces:files:delete'
		)
	),
	PRIMARY KEY (api_key_id, scope)
);

CREATE INDEX IF NOT EXISTS idx_api_key_scopes_scope
	ON api_key_scopes(scope);

CREATE TABLE IF NOT EXISTS api_key_workspaces (
	api_key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
	workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	PRIMARY KEY (api_key_id, workspace_id)
);

CREATE INDEX IF NOT EXISTS idx_api_key_workspaces_workspace_id
	ON api_key_workspaces(workspace_id);

COMMIT;