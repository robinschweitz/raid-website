ALTER TABLE loot_awards ADD COLUMN withdrawn_at DATETIME;
ALTER TABLE loot_awards ADD COLUMN withdrawn_by_player_id INTEGER REFERENCES players(id);
ALTER TABLE loot_awards ADD COLUMN withdrawn_reason TEXT;

CREATE INDEX idx_loot_active_main_spec_player
ON loot_awards(raid_id, awarded_to_player_id)
WHERE is_main_spec = 1 AND withdrawn_at IS NULL;

-- legacy_raid_mappings is immutable, so link metadata has its own append-only table.
CREATE TABLE legacy_raid_sync_links (
    source TEXT NOT NULL,
    sheet_id TEXT NOT NULL,
    raid_key TEXT NOT NULL,
    linked_by_player_id INTEGER NOT NULL REFERENCES players(id),
    linked_at DATETIME NOT NULL,
    link_reason TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (source, sheet_id, raid_key),
    FOREIGN KEY (source, sheet_id, raid_key)
        REFERENCES legacy_raid_mappings(source, sheet_id, raid_key)
);
CREATE TRIGGER legacy_raid_sync_links_no_update BEFORE UPDATE ON legacy_raid_sync_links
BEGIN SELECT RAISE(ABORT, 'legacy raid sync links are append-only'); END;
CREATE TRIGGER legacy_raid_sync_links_no_delete BEFORE DELETE ON legacy_raid_sync_links
BEGIN SELECT RAISE(ABORT, 'legacy raid sync links are append-only'); END;

CREATE TABLE legacy_loot_sync_states (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,
    sheet_id TEXT NOT NULL,
    raid_key TEXT NOT NULL,
    raw_hash TEXT NOT NULL,
    synced_by_player_id INTEGER NOT NULL REFERENCES players(id),
    synced_at DATETIME NOT NULL,
    UNIQUE (source, sheet_id, raid_key, raw_hash),
    FOREIGN KEY (source, sheet_id, raid_key)
        REFERENCES legacy_raid_mappings(source, sheet_id, raid_key)
);
CREATE INDEX idx_legacy_loot_sync_states_raid
ON legacy_loot_sync_states(source, sheet_id, raid_key, id DESC);
CREATE TRIGGER legacy_loot_sync_states_no_update BEFORE UPDATE ON legacy_loot_sync_states
BEGIN SELECT RAISE(ABORT, 'legacy loot sync states are append-only'); END;
CREATE TRIGGER legacy_loot_sync_states_no_delete BEFORE DELETE ON legacy_loot_sync_states
BEGIN SELECT RAISE(ABORT, 'legacy loot sync states are append-only'); END;

CREATE TABLE legacy_loot_sync_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    sync_state_id INTEGER NOT NULL REFERENCES legacy_loot_sync_states(id),
    legacy_loot_mapping_id INTEGER NOT NULL REFERENCES legacy_loot_mappings(id),
    loot_award_id INTEGER NOT NULL REFERENCES loot_awards(id),
    event_type TEXT NOT NULL CHECK (event_type IN ('CORRECTED', 'WITHDRAWN', 'RESTORED')),
    actor_id INTEGER NOT NULL REFERENCES players(id),
    reason TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL
);
CREATE INDEX idx_legacy_loot_sync_events_mapping
ON legacy_loot_sync_events(legacy_loot_mapping_id, id DESC);
CREATE INDEX idx_legacy_loot_sync_events_award
ON legacy_loot_sync_events(loot_award_id, id DESC);
CREATE TRIGGER legacy_loot_sync_events_no_update BEFORE UPDATE ON legacy_loot_sync_events
BEGIN SELECT RAISE(ABORT, 'legacy loot sync events are append-only'); END;
CREATE TRIGGER legacy_loot_sync_events_no_delete BEFORE DELETE ON legacy_loot_sync_events
BEGIN SELECT RAISE(ABORT, 'legacy loot sync events are append-only'); END;
