-- Loot that went to nobody: disenchanted, for the guild bank, or nobody wanted
-- it (ADR 089). Never an award: it counts for no player and costs no EP.
CREATE TABLE unclaimed_loot (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    raid_id INTEGER NOT NULL REFERENCES raids(id),
    catalog_boss_id TEXT REFERENCES loot_catalog_bosses(id),
    boss TEXT NOT NULL DEFAULT '',
    item_id TEXT NOT NULL DEFAULT '',
    item_name TEXT NOT NULL,
    outcome TEXT NOT NULL CHECK (outcome IN ('DISENCHANTED', 'BANK', 'NOBODY')),
    holder_name TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL CHECK (source IN ('manual', 'addon')),
    addon_line_key TEXT,
    created_by_player_id INTEGER NOT NULL REFERENCES players(id),
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    archived_at TIMESTAMP,
    archived_by_player_id INTEGER REFERENCES players(id),
    archive_reason TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_unclaimed_loot_raid ON unclaimed_loot(raid_id);
CREATE UNIQUE INDEX idx_unclaimed_loot_addon_line ON unclaimed_loot(raid_id, addon_line_key) WHERE addon_line_key IS NOT NULL;
