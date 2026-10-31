-- name: CreatePasswordCredentials :one
INSERT INTO password_credentials (user_id, password_hash, password_set_by)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetPasswordCredentialsByUserId :one
SELECT *
FROM password_credentials
WHERE user_id = $1;

-- name: UpdatePasswordCredentials :one
UPDATE password_credentials
SET password_hash = $2,
    password_set_by = $3,
    updated_at = now()
WHERE user_id = $1
RETURNING *;

-- name: CreateEmailVerificationChallenge :one
INSERT INTO email_verification_challenges (email, user_id, purpose, code_hash, expires_at, requested_by_ip, request_context, resend_available_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetEmailVerificationChallengeById :one
SELECT * FROM email_verification_challenges
WHERE id = $1;

-- name: ConsumeActiveEmailVerificationChallengesByUserAndPurpose :exec
UPDATE email_verification_challenges
SET consumed_at = now()
WHERE user_id = $1
    AND purpose = $2
    AND consumed_at IS NULL;

-- name: DeleteEmailVerificationChallengeById :exec
DELETE FROM email_verification_challenges
WHERE id = $1;

-- name: GetEmailVerificationCode :one
SELECT * FROM email_verification_challenges
WHERE email = $1 AND purpose = $2 AND consumed_at IS NULL
ORDER BY created_at DESC
LIMIT 1;

-- name: ConsumeEmailVerificationChallenge :one
UPDATE email_verification_challenges
SET consumed_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateResendAvailableAt :one
UPDATE email_verification_challenges
SET resend_available_at = $2
WHERE id = $1
RETURNING *;

-- name: CreateAuthRateLimit :one
INSERT INTO auth_rate_limits (key, scope, attempt_count)
VALUES ($1, $2, $3)
ON CONFLICT (key, scope) DO UPDATE
SET key = EXCLUDED.key
RETURNING *;

-- name: IncrementAuthRateLimitAttemptCount :one
UPDATE auth_rate_limits
SET attempt_count = attempt_count + 1,
    updated_at = now()
WHERE key = $1 AND scope = $2
RETURNING *;

-- name: GetAuthRateLimitByKeyAndScope :one
SELECT * FROM auth_rate_limits
WHERE key = $1 AND scope = $2;

-- name: BlockUntil :one
UPDATE auth_rate_limits
SET blocked_until = $3,
    updated_at = now()
WHERE key = $1 AND scope = $2
RETURNING *;

-- name: ConsumeAuthRateLimitAttempt :one
INSERT INTO auth_rate_limits (key, scope, attempt_count)
VALUES ($1, $2, 1)
ON CONFLICT (key, scope) DO UPDATE
SET attempt_count = CASE
        WHEN auth_rate_limits.blocked_until > now() THEN auth_rate_limits.attempt_count
        WHEN auth_rate_limits.updated_at <= sqlc.arg(window_starts_at)::timestamp THEN 1
        ELSE auth_rate_limits.attempt_count + 1
    END,
    blocked_until = CASE
        WHEN auth_rate_limits.blocked_until > now() THEN auth_rate_limits.blocked_until
        WHEN auth_rate_limits.updated_at <= sqlc.arg(window_starts_at)::timestamp THEN NULL
        WHEN auth_rate_limits.attempt_count >= sqlc.arg(max_attempts) THEN sqlc.arg(blocked_until)::timestamptz
        ELSE NULL
    END,
    updated_at = CASE
        WHEN auth_rate_limits.blocked_until > now() THEN auth_rate_limits.updated_at
        ELSE now()
    END
RETURNING *;

-- name: ResetAuthRateLimit :exec
DELETE FROM auth_rate_limits
WHERE key = $1 AND scope = $2;
