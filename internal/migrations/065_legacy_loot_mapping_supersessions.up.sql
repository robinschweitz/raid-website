CREATE TABLE legacy_loot_mapping_supersessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,
    sheet_id TEXT NOT NULL,
    legacy_loot_mapping_id INTEGER NOT NULL UNIQUE REFERENCES legacy_loot_mappings(id),
    successor_loot_mapping_id INTEGER NOT NULL UNIQUE REFERENCES legacy_loot_mappings(id),
    actor_id INTEGER NOT NULL REFERENCES players(id),
    reason TEXT NOT NULL,
    created_at DATETIME NOT NULL
);
CREATE INDEX idx_legacy_loot_mapping_supersessions_scope
ON legacy_loot_mapping_supersessions(source, sheet_id, id);
CREATE TRIGGER legacy_loot_mapping_supersessions_no_update BEFORE UPDATE ON legacy_loot_mapping_supersessions
BEGIN SELECT RAISE(ABORT, 'legacy loot mapping supersessions are append-only'); END;
CREATE TRIGGER legacy_loot_mapping_supersessions_no_delete BEFORE DELETE ON legacy_loot_mapping_supersessions
BEGIN SELECT RAISE(ABORT, 'legacy loot mapping supersessions are append-only'); END;
