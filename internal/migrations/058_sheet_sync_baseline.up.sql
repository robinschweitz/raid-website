CREATE TABLE sheet_source_baselines (
  source TEXT NOT NULL,
  sheet_id TEXT NOT NULL,
  data_json TEXT NOT NULL,
  PRIMARY KEY (source, sheet_id)
);
CREATE TABLE sheet_setup_baselines (
  source TEXT NOT NULL,
  sheet_id TEXT NOT NULL,
  raid_id INTEGER NOT NULL REFERENCES raids(id),
  setup_csv TEXT NOT NULL,
  PRIMARY KEY (source, sheet_id, raid_id)
);
