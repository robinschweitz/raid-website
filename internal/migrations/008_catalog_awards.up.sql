ALTER TABLE loot_awards ADD COLUMN catalog_boss_id TEXT REFERENCES loot_catalog_bosses(id);
