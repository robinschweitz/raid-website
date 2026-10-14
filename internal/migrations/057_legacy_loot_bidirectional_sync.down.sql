DROP TRIGGER legacy_loot_sync_events_no_delete;
DROP TRIGGER legacy_loot_sync_events_no_update;
DROP INDEX idx_legacy_loot_sync_events_award;
DROP INDEX idx_legacy_loot_sync_events_mapping;
DROP TABLE legacy_loot_sync_events;

DROP TRIGGER legacy_loot_sync_states_no_delete;
DROP TRIGGER legacy_loot_sync_states_no_update;
DROP INDEX idx_legacy_loot_sync_states_raid;
DROP TABLE legacy_loot_sync_states;

DROP TRIGGER legacy_raid_sync_links_no_delete;
DROP TRIGGER legacy_raid_sync_links_no_update;
DROP TABLE legacy_raid_sync_links;

DROP INDEX idx_loot_active_main_spec_player;
ALTER TABLE loot_awards DROP COLUMN withdrawn_reason;
ALTER TABLE loot_awards DROP COLUMN withdrawn_by_player_id;
ALTER TABLE loot_awards DROP COLUMN withdrawn_at;
