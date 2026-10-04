CREATE TABLE loot_catalog_initializations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  source_hash TEXT NOT NULL,
  report TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX idx_loot_catalog_initializations_hash ON loot_catalog_initializations(source_hash);
