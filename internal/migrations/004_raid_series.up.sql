CREATE TABLE raid_series (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  raid_leader_id INTEGER NOT NULL REFERENCES players(id),
  weekdays TEXT NOT NULL,
  interval_weeks INTEGER NOT NULL CHECK (interval_weeks > 0),
  start_time TEXT NOT NULL,
  timezone TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
ALTER TABLE raids ADD COLUMN series_id INTEGER REFERENCES raid_series(id);
ALTER TABLE raids ADD COLUMN previous_raid_id INTEGER REFERENCES raids(id);
CREATE UNIQUE INDEX idx_raids_series_occurrence ON raids(series_id, scheduled_start);
CREATE UNIQUE INDEX idx_raids_previous ON raids(previous_raid_id);
