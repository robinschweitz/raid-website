DROP INDEX idx_raids_previous;
DROP INDEX idx_raids_series_occurrence;
ALTER TABLE raids DROP COLUMN previous_raid_id;
ALTER TABLE raids DROP COLUMN series_id;
DROP TABLE raid_series;
