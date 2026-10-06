ALTER TABLE raid_series ADD COLUMN name_template TEXT;
ALTER TABLE raid_series ADD COLUMN next_raid_number INTEGER;
UPDATE raid_series SET name_template = name, next_raid_number = 1 WHERE name_template IS NULL;
