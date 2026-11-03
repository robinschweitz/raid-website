-- ADR 112. Further series leaders after raid_series.raid_leader_id, which stays the first one.
CREATE TABLE raid_series_leaders (
  series_id INTEGER NOT NULL REFERENCES raid_series(id) ON DELETE CASCADE,
  player_id INTEGER NOT NULL REFERENCES players(id),
  position INTEGER NOT NULL,
  PRIMARY KEY (series_id, player_id)
);
CREATE INDEX raid_series_leaders_player ON raid_series_leaders(player_id);
