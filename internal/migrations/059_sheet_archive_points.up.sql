CREATE TABLE sheet_point_baselines (
  source TEXT NOT NULL,
  sheet_id TEXT NOT NULL,
  player_id INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  transaction_id INTEGER NOT NULL,
  PRIMARY KEY (source, sheet_id, player_id)
);
CREATE TABLE sheet_provenance_scopes (
  source TEXT NOT NULL,
  sheet_id TEXT NOT NULL,
  from_sheet_id TEXT NOT NULL,
  PRIMARY KEY (source, sheet_id)
);
CREATE TRIGGER sheet_provenance_scopes_no_update BEFORE UPDATE ON sheet_provenance_scopes
BEGIN SELECT RAISE(ABORT, 'sheet provenance is append-only'); END;
CREATE TRIGGER sheet_provenance_scopes_no_delete BEFORE DELETE ON sheet_provenance_scopes
BEGIN SELECT RAISE(ABORT, 'sheet provenance is append-only'); END;
