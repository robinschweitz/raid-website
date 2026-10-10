ALTER TABLE characters ADD COLUMN external_source TEXT NOT NULL DEFAULT '';
ALTER TABLE characters ADD COLUMN external_profile_id TEXT NOT NULL DEFAULT '';
ALTER TABLE characters ADD COLUMN external_profile_url TEXT NOT NULL DEFAULT '';
ALTER TABLE characters ADD COLUMN external_profile_json TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_characters_external_profile ON characters(external_source, external_profile_id) WHERE external_source <> '' AND external_profile_id <> '';
