-- 084 is the already deployed raid-news migration; 085 is reserved for loot difficulty.
CREATE TABLE password_reset_codes (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 player_id INTEGER NOT NULL REFERENCES players(id),
 code_hash TEXT NOT NULL UNIQUE,
 created_by_player_id INTEGER REFERENCES players(id),
 created_at DATETIME NOT NULL,
 expires_at DATETIME NOT NULL,
 used_at DATETIME,
 revoked_at DATETIME
);
CREATE INDEX password_reset_codes_player ON password_reset_codes(player_id);
