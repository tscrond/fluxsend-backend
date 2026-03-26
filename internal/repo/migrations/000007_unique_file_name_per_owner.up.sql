-- First, drop the existing unique constraint to allow deduplication
ALTER TABLE files DROP CONSTRAINT files_unique;

-- Deduplicate existing rows: keep the highest-id row per (owner, filename)
DELETE FROM files
WHERE id NOT IN (
    SELECT MAX(id)
    FROM files
    GROUP BY owner_google_id, file_name
);

-- Add the new unique constraint on (owner_google_id, file_name)
-- This ensures a file path must be unique per user, matching real cloud-storage semantics
ALTER TABLE files ADD CONSTRAINT files_unique_per_owner UNIQUE (owner_google_id, file_name);
