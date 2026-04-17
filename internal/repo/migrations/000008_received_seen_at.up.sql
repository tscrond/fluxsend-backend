ALTER TABLE shares ADD COLUMN received_seen_at TIMESTAMPTZ DEFAULT NULL;

-- Mark all existing shares as seen to avoid badge flood on first deploy
UPDATE shares SET received_seen_at = NOW() WHERE received_seen_at IS NULL;
