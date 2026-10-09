CREATE TABLE IF NOT EXISTS token_transactions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  player_id INTEGER NOT NULL,
  raid_id INTEGER,
  loot_award_id INTEGER,
  amount INTEGER NOT NULL,
  type TEXT NOT NULL,
  reason TEXT NOT NULL,
  ref_type TEXT NOT NULL DEFAULT '',
  ref_id TEXT NOT NULL DEFAULT '',
  actor_id INTEGER NOT NULL,
  created_at DATETIME NOT NULL,
  FOREIGN KEY (player_id) REFERENCES players(id),
  FOREIGN KEY (raid_id) REFERENCES raids(id),
  FOREIGN KEY (loot_award_id) REFERENCES loot_awards(id)
);
CREATE INDEX IF NOT EXISTS idx_tok_player_created ON token_transactions(player_id, created_at);
CREATE INDEX IF NOT EXISTS idx_tok_raid ON token_transactions(raid_id);
