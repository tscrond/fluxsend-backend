CREATE TABLE password_credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    password_hash TEXT NOT NULL,
    password_set_by TEXT NOT NULL CHECK (password_set_by IN ('self_register', 'oauth_attach', 'password_reset')),
    last_password_login_verified_at TIMESTAMPTZ NULL,
    updated_at TIMESTAMP DEFAULT now(),
    created_at TIMESTAMP DEFAULT now()
);

CREATE TABLE email_verification_challenges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose TEXT NOT NULL CHECK (purpose IN ('register', 'password_login_step_up', 'password_reset', 'password_attach')),
    code_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    max_attempts INT NOT NULL DEFAULT 5,
    resend_available_at TIMESTAMPTZ NOT NULL,
    requested_by_ip TEXT NOT NULL,
    request_context JSONB NOT NULL,
    consumed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMP DEFAULT now()
);

CREATE TABLE auth_rate_limits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key TEXT NOT NULL,
    scope TEXT NOT NULL CHECK (scope IN ('register_send', 'login_password', 'login_code_verify','password_reset_send','password_reset_verify')),
    attempt_count INT NOT NULL DEFAULT 0,
    blocked_until TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);
