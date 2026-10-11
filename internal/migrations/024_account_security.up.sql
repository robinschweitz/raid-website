CREATE TABLE IF NOT EXISTS login_attempts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL,
  client_ip TEXT NOT NULL,
  success BOOLEAN NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_login_attempts_lookup ON login_attempts(username, client_ip, created_at);

CREATE TABLE IF NOT EXISTS player_mfa_settings (
  player_id INTEGER PRIMARY KEY,
  totp_secret TEXT NOT NULL DEFAULT '',
  totp_enabled_at DATETIME,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (player_id) REFERENCES players(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS mfa_recovery_codes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  player_id INTEGER NOT NULL,
  code_hash TEXT NOT NULL UNIQUE,
  used_at DATETIME,
  created_at DATETIME NOT NULL,
  FOREIGN KEY (player_id) REFERENCES players(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_mfa_recovery_codes_player ON mfa_recovery_codes(player_id, used_at);

CREATE TABLE IF NOT EXISTS mfa_login_challenges (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  challenge_hash TEXT NOT NULL UNIQUE,
  player_id INTEGER NOT NULL,
  created_at DATETIME NOT NULL,
  expires_at DATETIME NOT NULL,
  used_at DATETIME,
  FOREIGN KEY (player_id) REFERENCES players(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_mfa_login_challenges_hash ON mfa_login_challenges(challenge_hash, expires_at);

CREATE TABLE IF NOT EXISTS webauthn_credentials (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  player_id INTEGER NOT NULL,
  credential_id_hash TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL DEFAULT '',
  credential_json TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  last_used_at DATETIME,
  FOREIGN KEY (player_id) REFERENCES players(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_webauthn_credentials_player ON webauthn_credentials(player_id, created_at);

CREATE TABLE IF NOT EXISTS webauthn_sessions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_hash TEXT NOT NULL UNIQUE,
  player_id INTEGER NOT NULL,
  mfa_challenge_id INTEGER,
  purpose TEXT NOT NULL,
  session_json TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  expires_at DATETIME NOT NULL,
  used_at DATETIME,
  FOREIGN KEY (player_id) REFERENCES players(id) ON DELETE CASCADE,
  FOREIGN KEY (mfa_challenge_id) REFERENCES mfa_login_challenges(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_webauthn_sessions_hash ON webauthn_sessions(session_hash, expires_at);
