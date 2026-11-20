CREATE TABLE support_views (
  session_hash TEXT PRIMARY KEY REFERENCES sessions(token_hash) ON DELETE CASCADE,
  view_id TEXT NOT NULL UNIQUE,
  target_id INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  writable BOOLEAN NOT NULL DEFAULT FALSE
);
ALTER TABLE audit_log ADD COLUMN represented_player_id INTEGER;
