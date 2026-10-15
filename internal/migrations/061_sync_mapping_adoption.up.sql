CREATE TABLE legacy_sync_raid_mappings (
    source TEXT NOT NULL,
    sheet_id TEXT NOT NULL,
    raid_key TEXT NOT NULL,
    raid_id INTEGER NOT NULL REFERENCES raids(id),
    adopted_by_player_id INTEGER NOT NULL REFERENCES players(id),
    adopted_at DATETIME NOT NULL,
    PRIMARY KEY (source, sheet_id, raid_key),
    UNIQUE (source, sheet_id, raid_id)
);
CREATE TABLE legacy_sync_player_mappings (
    source TEXT NOT NULL,
    sheet_id TEXT NOT NULL,
    player_name TEXT NOT NULL,
    player_id INTEGER NOT NULL REFERENCES players(id),
    adopted_by_player_id INTEGER NOT NULL REFERENCES players(id),
    adopted_at DATETIME NOT NULL,
    PRIMARY KEY (source, sheet_id, player_name)
);
CREATE TABLE legacy_sync_character_mappings (
    source TEXT NOT NULL,
    sheet_id TEXT NOT NULL,
    character_name TEXT NOT NULL,
    player_name TEXT NOT NULL,
    character_id INTEGER NOT NULL REFERENCES characters(id),
    player_id INTEGER NOT NULL REFERENCES players(id),
    adopted_by_player_id INTEGER NOT NULL REFERENCES players(id),
    adopted_at DATETIME NOT NULL,
    PRIMARY KEY (source, sheet_id, character_name)
);

-- Sync links can target raids already owned by another immutable import mapping.
-- Rebuild the state/event pair because SQLite cannot replace the old composite FK.
PRAGMA foreign_keys = OFF;
CREATE TABLE legacy_loot_sync_states_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,
    sheet_id TEXT NOT NULL,
    raid_key TEXT NOT NULL,
    raw_hash TEXT NOT NULL,
    synced_by_player_id INTEGER NOT NULL REFERENCES players(id),
    synced_at DATETIME NOT NULL,
    sync_raid_id INTEGER REFERENCES raids(id),
    UNIQUE (source, sheet_id, raid_key, raw_hash)
);
INSERT INTO legacy_loot_sync_states_new (id, source, sheet_id, raid_key, raw_hash, synced_by_player_id, synced_at)
SELECT id, source, sheet_id, raid_key, raw_hash, synced_by_player_id, synced_at FROM legacy_loot_sync_states;
CREATE TABLE legacy_loot_sync_events_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    sync_state_id INTEGER NOT NULL REFERENCES legacy_loot_sync_states_new(id),
    legacy_loot_mapping_id INTEGER NOT NULL REFERENCES legacy_loot_mappings(id),
    loot_award_id INTEGER NOT NULL REFERENCES loot_awards(id),
    event_type TEXT NOT NULL CHECK (event_type IN ('CORRECTED', 'WITHDRAWN', 'RESTORED')),
    actor_id INTEGER NOT NULL REFERENCES players(id),
    reason TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL
);
INSERT INTO legacy_loot_sync_events_new SELECT * FROM legacy_loot_sync_events;
DROP TABLE legacy_loot_sync_events;
DROP TABLE legacy_loot_sync_states;
ALTER TABLE legacy_loot_sync_states_new RENAME TO legacy_loot_sync_states;
ALTER TABLE legacy_loot_sync_events_new RENAME TO legacy_loot_sync_events;
CREATE INDEX idx_legacy_loot_sync_states_raid ON legacy_loot_sync_states(source, sheet_id, raid_key, id DESC);
CREATE INDEX idx_legacy_loot_sync_events_mapping ON legacy_loot_sync_events(legacy_loot_mapping_id, id DESC);
CREATE INDEX idx_legacy_loot_sync_events_award ON legacy_loot_sync_events(loot_award_id, id DESC);
CREATE TRIGGER legacy_loot_sync_states_no_update BEFORE UPDATE ON legacy_loot_sync_states BEGIN SELECT RAISE(ABORT, 'legacy loot sync states are append-only'); END;
CREATE TRIGGER legacy_loot_sync_states_no_delete BEFORE DELETE ON legacy_loot_sync_states BEGIN SELECT RAISE(ABORT, 'legacy loot sync states are append-only'); END;
CREATE TRIGGER legacy_loot_sync_events_no_update BEFORE UPDATE ON legacy_loot_sync_events BEGIN SELECT RAISE(ABORT, 'legacy loot sync events are append-only'); END;
CREATE TRIGGER legacy_loot_sync_events_no_delete BEFORE DELETE ON legacy_loot_sync_events BEGIN SELECT RAISE(ABORT, 'legacy loot sync events are append-only'); END;
PRAGMA foreign_keys = ON;
