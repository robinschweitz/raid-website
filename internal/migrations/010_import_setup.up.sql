CREATE TABLE legacy_setup_mappings (
  source TEXT NOT NULL,
  sheet_id TEXT NOT NULL,
  series_id INTEGER NOT NULL REFERENCES raid_series(id),
  raid_id INTEGER NOT NULL REFERENCES raids(id),
  PRIMARY KEY (source, sheet_id),
  UNIQUE (series_id),
  UNIQUE (raid_id)
);
CREATE TRIGGER legacy_setup_mappings_no_update BEFORE UPDATE ON legacy_setup_mappings
BEGIN SELECT RAISE(ABORT, 'setup provenance is append-only'); END;
CREATE TRIGGER legacy_setup_mappings_no_delete BEFORE DELETE ON legacy_setup_mappings
BEGIN SELECT RAISE(ABORT, 'setup provenance is append-only'); END;
