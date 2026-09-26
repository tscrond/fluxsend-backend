ALTER TABLE email_verification_challenges
DROP CONSTRAINT IF EXISTS email_verification_challenges_purpose_check;

ALTER TABLE email_verification_challenges
ADD CONSTRAINT email_verification_challenges_purpose_check CHECK (
    purpose IN ('register', 'password_login_step_up', 'password_reset', 'password_attach', 'password_reset_self')
);

ALTER TABLE auth_rate_limits
DROP CONSTRAINT IF EXISTS auth_rate_limits_scope_check;

ALTER TABLE auth_rate_limits
ADD CONSTRAINT auth_rate_limits_scope_check CHECK (
    scope IN (
        'register_send',
        'login_password',
        'login_code_verify',
        'password_reset_send',
        'password_reset_verify',
        'password_reset_self_init'
    )
);