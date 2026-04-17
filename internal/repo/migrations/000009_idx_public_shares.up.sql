CREATE UNIQUE INDEX idx_public_shares ON shares(shared_by, file_id) WHERE shared_for IS NULL;
