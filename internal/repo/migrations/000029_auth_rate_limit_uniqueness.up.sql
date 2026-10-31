WITH duplicate_rate_limits AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY key, scope
               ORDER BY (blocked_until IS NOT NULL AND blocked_until > now()) DESC,
                        updated_at DESC NULLS LAST,
                        id
           ) AS row_number
    FROM auth_rate_limits
)
DELETE FROM auth_rate_limits
WHERE id IN (
    SELECT id
    FROM duplicate_rate_limits
    WHERE row_number > 1
);

CREATE UNIQUE INDEX auth_rate_limits_key_scope_idx
    ON auth_rate_limits (key, scope);
