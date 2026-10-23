ALTER TABLE mfa_login_challenges ADD COLUMN failed_attempts INTEGER NOT NULL DEFAULT 0;
