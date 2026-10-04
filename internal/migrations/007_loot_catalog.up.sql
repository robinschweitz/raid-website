CREATE TABLE loot_catalog_sources (
  source_id TEXT NOT NULL,
  revision TEXT NOT NULL,
  raw_hash TEXT NOT NULL UNIQUE,
  metadata TEXT NOT NULL,
  PRIMARY KEY (source_id, revision)
);
CREATE TABLE loot_catalog_instances (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  source_id TEXT NOT NULL
);
CREATE TABLE loot_catalog_bosses (
  id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL REFERENCES loot_catalog_instances(id),
  name TEXT NOT NULL,
  source_id TEXT NOT NULL
);
CREATE INDEX idx_loot_catalog_boss_instance ON loot_catalog_bosses(instance_id);
CREATE TABLE loot_catalog_items (
  item_id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  binding INTEGER,
  quality INTEGER,
  source_id TEXT NOT NULL
);
CREATE TABLE loot_catalog_drops (
  boss_id TEXT NOT NULL REFERENCES loot_catalog_bosses(id),
  item_id TEXT NOT NULL REFERENCES loot_catalog_items(item_id),
  raid_size INTEGER NOT NULL CHECK (raid_size IN (10, 25)),
  difficulty TEXT NOT NULL,
  loot_mode INTEGER NOT NULL CHECK (loot_mode BETWEEN 1 AND 65535),
  source_id TEXT NOT NULL,
  PRIMARY KEY (boss_id, item_id, raid_size, difficulty, loot_mode)
);
CREATE TABLE loot_catalog_previews (
  source_id TEXT NOT NULL,
  revision TEXT NOT NULL,
  raw_hash TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (source_id, revision, raw_hash)
);
ALTER TABLE raids ADD COLUMN catalog_instance_id TEXT REFERENCES loot_catalog_instances(id);
ALTER TABLE raids ADD COLUMN raid_size INTEGER CHECK (raid_size IS NULL OR raid_size IN (10, 25));
ALTER TABLE raid_series ADD COLUMN catalog_instance_id TEXT REFERENCES loot_catalog_instances(id);
ALTER TABLE raid_series ADD COLUMN raid_size INTEGER CHECK (raid_size IS NULL OR raid_size IN (10, 25));
