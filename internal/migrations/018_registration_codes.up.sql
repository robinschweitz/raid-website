CREATE TABLE IF NOT EXISTS registration_requests (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  requester_player_id INTEGER,
  status TEXT NOT NULL,
  purpose TEXT NOT NULL,
  external_source TEXT NOT NULL,
  external_profile_id TEXT NOT NULL,
  external_profile_url TEXT NOT NULL,
  character_name TEXT NOT NULL,
  class TEXT NOT NULL,
  main_spec TEXT NOT NULL,
  second_spec TEXT NOT NULL DEFAULT '',
  level INTEGER NOT NULL DEFAULT 0,
  race TEXT NOT NULL DEFAULT '',
  faction TEXT NOT NULL DEFAULT '',
  raw_profile_json TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (requester_player_id) REFERENCES players(id)
);
CREATE INDEX IF NOT EXISTS idx_registration_requests_status ON registration_requests(status, created_at);
CREATE INDEX IF NOT EXISTS idx_registration_requests_profile ON registration_requests(external_source, external_profile_id);

CREATE TABLE IF NOT EXISTS registration_codes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  request_id INTEGER NOT NULL,
  code_hash TEXT NOT NULL UNIQUE,
  created_by_player_id INTEGER NOT NULL,
  created_at DATETIME NOT NULL,
  expires_at DATETIME NOT NULL,
  used_at DATETIME,
  used_by_player_id INTEGER,
  revoked_at DATETIME,
  FOREIGN KEY (request_id) REFERENCES registration_requests(id),
  FOREIGN KEY (created_by_player_id) REFERENCES players(id),
  FOREIGN KEY (used_by_player_id) REFERENCES players(id)
);
CREATE INDEX IF NOT EXISTS idx_registration_codes_request ON registration_codes(request_id, created_at);
CREATE INDEX IF NOT EXISTS idx_registration_codes_active ON registration_codes(code_hash, expires_at) WHERE used_at IS NULL AND revoked_at IS NULL;
