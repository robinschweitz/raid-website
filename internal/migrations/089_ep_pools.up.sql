-- ADR 113. EP pools: every transaction and lock belongs to one pool; series and
-- raids choose a pool (or none) and an EP award. All existing data joins "ICC".
CREATE TABLE ep_pools (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
INSERT INTO ep_pools (id, name, created_at, updated_at) VALUES (1, 'ICC', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

ALTER TABLE enhance_point_transactions ADD COLUMN pool_id INTEGER NOT NULL DEFAULT 1;
CREATE INDEX enhance_point_transactions_pool_player ON enhance_point_transactions(pool_id, player_id);

ALTER TABLE item_locks ADD COLUMN pool_id INTEGER NOT NULL DEFAULT 1;

-- Series and raids default to the first pool and +10, as before ADR 113; NULL
-- means "no pool". Pools are never deleted, so no foreign key is needed here
-- (SQLite cannot add one with a non-NULL default).
ALTER TABLE raid_series ADD COLUMN ep_pool_id INTEGER DEFAULT 1;
ALTER TABLE raid_series ADD COLUMN ep_award INTEGER NOT NULL DEFAULT 10;

ALTER TABLE raids ADD COLUMN ep_pool_id INTEGER DEFAULT 1;
ALTER TABLE raids ADD COLUMN ep_award INTEGER NOT NULL DEFAULT 10;
