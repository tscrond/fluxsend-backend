BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS plans(
  id                      UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
  name                    TEXT    NOT NULL UNIQUE,
  max_total_storage_bytes BIGINT  NOT NULL,
  max_file_size_bytes     BIGINT  NOT NULL,
  max_files               INT     NOT NULL,
  max_files_sent_per_day  INT     NOT NULL,
  max_shares_per_day      INT     NOT NULL,
  created_at              TIMESTAMPTZ DEFAULT now() NOT NULL
);

CREATE TABLE IF NOT EXISTS plan_features(
  plan_id UUID NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
  feature TEXT NOT NULL,
  PRIMARY KEY (plan_id, feature)
);

-- ----------------------------------------------------------------
-- Seed plans
-- ----------------------------------------------------------------
INSERT INTO plans (id, name, max_total_storage_bytes, max_file_size_bytes, max_files, max_files_sent_per_day, max_shares_per_day)
VALUES
  -- 5 GB total, 250 MB per file, 20 files
  ('00000000-0000-0000-0000-000000000001', 'free',       5368709120,    262144000,    20,   5,    10),
  -- 50 GB total, 2 GB per file, 500 files
  ('00000000-0000-0000-0000-000000000002', 'developer',  53687091200,   2147483648,   500,  100,  500),
  -- 1 TB total, 10 GB per file, unlimited files (INT max used as sentinel)
  ('00000000-0000-0000-0000-000000000003', 'enterprise', 1099511627776, 10737418240,  2147483647, 2147483647, 2147483647);

-- ----------------------------------------------------------------
-- Seed features — all plans get the full base feature set for now
-- ----------------------------------------------------------------
INSERT INTO plan_features (plan_id, feature)
SELECT p.id, f.feature
FROM plans p
CROSS JOIN (VALUES
  ('file_upload'),
  ('file_share'),
  ('quick_share'),
  ('file_notes'),
  ('private_download'),
  ('public_download')
) AS f(feature)
WHERE p.name IN ('free', 'developer', 'enterprise');

ALTER TABLE users ADD COLUMN plan_id UUID DEFAULT '00000000-0000-0000-0000-000000000001' REFERENCES plans(id) ON DELETE SET NULL;
UPDATE users SET plan_id = '00000000-0000-0000-0000-000000000001' WHERE plan_id IS NULL;

COMMIT;