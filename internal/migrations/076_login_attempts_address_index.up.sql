CREATE INDEX IF NOT EXISTS idx_login_attempts_address ON login_attempts(client_ip, created_at);
