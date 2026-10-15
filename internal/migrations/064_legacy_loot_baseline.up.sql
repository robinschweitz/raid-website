CREATE TABLE legacy_loot_baselines (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,
    sheet_id TEXT NOT NULL,
    legacy_loot_mapping_id INTEGER NOT NULL UNIQUE REFERENCES legacy_loot_mappings(id),
    baseline_loot_mapping_id INTEGER NOT NULL UNIQUE REFERENCES legacy_loot_mappings(id),
    import_id TEXT NOT NULL,
    actor_id INTEGER NOT NULL REFERENCES players(id),
    reason TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    UNIQUE(source, sheet_id, import_id)
);
CREATE INDEX idx_legacy_loot_baselines_scope
ON legacy_loot_baselines(source, sheet_id, id);
CREATE TRIGGER legacy_loot_baselines_no_update BEFORE UPDATE ON legacy_loot_baselines
BEGIN SELECT RAISE(ABORT, 'legacy loot baselines are append-only'); END;
CREATE TRIGGER legacy_loot_baselines_no_delete BEFORE DELETE ON legacy_loot_baselines
BEGIN SELECT RAISE(ABORT, 'legacy loot baselines are append-only'); END;
