ALTER TABLE email_verification_challenges
ALTER COLUMN user_id DROP NOT NULL;

ALTER TABLE email_verification_challenges
ALTER COLUMN resend_available_at SET DEFAULT now() + interval '60 seconds';
