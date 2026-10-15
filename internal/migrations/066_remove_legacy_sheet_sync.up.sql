DELETE FROM audit_log
WHERE entity_type IN (
    'legacy_sync_mapping',
    'legacy_loot_baseline',
    'legacy_loot_mapping_supersession',
    'legacy_raid_sync_link'
);

DROP TABLE website_loot_delivery_events;
DROP TABLE website_loot_delivery_outbox;
DROP TABLE legacy_loot_sync_events;
DROP TABLE legacy_loot_sync_states;
DROP TABLE legacy_loot_mapping_supersessions;
DROP TABLE legacy_loot_baselines;
DROP TABLE legacy_raid_sync_links;
DROP TABLE legacy_sync_character_mappings;
DROP TABLE legacy_sync_player_mappings;
DROP TABLE legacy_sync_raid_mappings;
DROP TABLE legacy_setup_mappings;
DROP TABLE legacy_loot_mappings;
DROP TABLE import_previews;
DROP TABLE legacy_raid_mappings;
DROP TABLE legacy_imports;
