ALTER TABLE file_uploads
    ADD COLUMN workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    ADD COLUMN path TEXT NOT NULL DEFAULT '/';

CREATE INDEX file_uploads_workspace_id_idx
    ON file_uploads(workspace_id);