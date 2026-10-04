DROP INDEX idx_ep_loot_award;
ALTER TABLE enhance_point_transactions DROP COLUMN loot_award_id;
DROP INDEX idx_loot_request;
ALTER TABLE loot_awards DROP COLUMN request_hash;
ALTER TABLE loot_awards DROP COLUMN request_id;
ALTER TABLE loot_awards DROP COLUMN enhanced_cost;
ALTER TABLE loot_awards DROP COLUMN award_method;
