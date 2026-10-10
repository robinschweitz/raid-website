DROP INDEX IF EXISTS idx_characters_external_profile;
ALTER TABLE characters DROP COLUMN external_profile_json;
ALTER TABLE characters DROP COLUMN external_profile_url;
ALTER TABLE characters DROP COLUMN external_profile_id;
ALTER TABLE characters DROP COLUMN external_source;
