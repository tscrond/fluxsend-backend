ALTER TABLE email_verification_challenges
ALTER COLUMN user_id SET NOT NULL;

ALTER TABLE email_verification_challenges
ALTER COLUMN resend_available_at DROP DEFAULT;
