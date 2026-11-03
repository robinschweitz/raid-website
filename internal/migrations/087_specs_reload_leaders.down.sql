DROP TABLE IF EXISTS raid_leaders;
ALTER TABLE characters DROP COLUMN specs_chosen_at;
ALTER TABLE characters DROP COLUMN profile_reloaded_at;
ALTER TABLE raid_signups DROP COLUMN specs;
