-- ADR 109. Empty specs means a signup from before several specs per signup: its spec column is the only one.
ALTER TABLE raid_signups ADD COLUMN specs TEXT NOT NULL DEFAULT '';
ALTER TABLE characters ADD COLUMN profile_reloaded_at DATETIME;
ALTER TABLE characters ADD COLUMN specs_chosen_at DATETIME;
-- Further raid leaders after raids.raid_leader_id, which stays the first one.
CREATE TABLE raid_leaders (
  raid_id INTEGER NOT NULL REFERENCES raids(id) ON DELETE CASCADE,
  player_id INTEGER NOT NULL REFERENCES players(id),
  position INTEGER NOT NULL,
  PRIMARY KEY (raid_id, player_id)
);
CREATE INDEX raid_leaders_player ON raid_leaders(player_id);
