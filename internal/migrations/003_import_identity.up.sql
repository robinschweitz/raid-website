CREATE TABLE legacy_raid_mappings (
    source TEXT NOT NULL,
    sheet_id TEXT NOT NULL,
    raid_key TEXT NOT NULL,
    raid_id INTEGER NOT NULL REFERENCES raids(id),
    PRIMARY KEY(source, sheet_id, raid_key),
    UNIQUE(raid_id)
);
CREATE TRIGGER legacy_raid_mappings_no_update BEFORE UPDATE ON legacy_raid_mappings
BEGIN SELECT RAISE(ABORT, 'legacy raid mappings are append-only'); END;
CREATE TRIGGER legacy_raid_mappings_no_delete BEFORE DELETE ON legacy_raid_mappings
BEGIN SELECT RAISE(ABORT, 'legacy raid mappings are append-only'); END;
CREATE TABLE import_previews (
    source TEXT NOT NULL,
    sheet_id TEXT NOT NULL,
    raw_hash TEXT NOT NULL,
    PRIMARY KEY(source, sheet_id, raw_hash)
);
CREATE TABLE legacy_loot_mappings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,
    sheet_id TEXT NOT NULL,
    tab TEXT NOT NULL,
    identity TEXT NOT NULL,
    external_id TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    loot_award_id INTEGER NOT NULL REFERENCES loot_awards(id),
    UNIQUE(source, sheet_id, tab, identity)
);
CREATE INDEX legacy_import_scope ON legacy_imports(source, source_url, raw_hash);
CREATE TRIGGER legacy_loot_mappings_no_update BEFORE UPDATE ON legacy_loot_mappings
BEGIN SELECT RAISE(ABORT, 'legacy loot mappings are append-only'); END;
CREATE TRIGGER legacy_loot_mappings_no_delete BEFORE DELETE ON legacy_loot_mappings
BEGIN SELECT RAISE(ABORT, 'legacy loot mappings are append-only'); END;
