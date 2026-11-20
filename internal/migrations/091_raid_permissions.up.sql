CREATE TABLE raid_permission_policies (
  player_id INTEGER PRIMARY KEY REFERENCES players(id) ON DELETE CASCADE,
  allowed INTEGER NOT NULL CHECK (allowed BETWEEN 0 AND 15),
  assigned_only BOOLEAN NOT NULL
);
CREATE TABLE raid_substitutes (
  raid_id INTEGER NOT NULL REFERENCES raids(id) ON DELETE CASCADE,
  player_id INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  grantor_id INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  allowed INTEGER NOT NULL CHECK (allowed BETWEEN 1 AND 15),
  PRIMARY KEY (raid_id, player_id)
);
