BEGIN;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS workspace_files (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id   UUID   REFERENCES workspaces(id) ON DELETE CASCADE NOT NULL,
  uploaded_by    UUID   REFERENCES users(id) ON DELETE CASCADE NOT NULL,
  file_name      TEXT   NOT NULL,
  file_type      TEXT,
  size           BIGINT NOT NULL DEFAULT 0,
  md5_checksum   TEXT,
  path           TEXT   NOT NULL DEFAULT '/',
  created_at     TIMESTAMPTZ DEFAULT now() NOT NULL,
  CONSTRAINT unique_workspace_file UNIQUE (workspace_id, path, file_name)
);

COMMIT;