-- Killed bosses per raid with the difficulty of each kill (FR-29). Removed
-- kills stay as history (FR-31.1); at most one kill per boss is active. Loot
-- recorded before this table existed gets no kill (FR-29.10).
CREATE TABLE raid_boss_kills (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    raid_id INTEGER NOT NULL REFERENCES raids(id),
    catalog_boss_id TEXT NOT NULL REFERENCES loot_catalog_bosses(id),
    boss TEXT NOT NULL,
    difficulty TEXT NOT NULL CHECK (difficulty IN ('NORMAL', 'HEROIC', 'HARDMODE')),
    created_by_player_id INTEGER NOT NULL REFERENCES players(id),
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    removed_at TIMESTAMP,
    removed_by_player_id INTEGER REFERENCES players(id),
    remove_reason TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX idx_raid_boss_kills_active ON raid_boss_kills(raid_id, catalog_boss_id) WHERE removed_at IS NULL;
