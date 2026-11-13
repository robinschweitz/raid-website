DROP INDEX IF EXISTS enhance_point_transactions_pool_player;
ALTER TABLE raids DROP COLUMN ep_award;
ALTER TABLE raids DROP COLUMN ep_pool_id;
ALTER TABLE raid_series DROP COLUMN ep_award;
ALTER TABLE raid_series DROP COLUMN ep_pool_id;
ALTER TABLE item_locks DROP COLUMN pool_id;
ALTER TABLE enhance_point_transactions DROP COLUMN pool_id;
DROP TABLE ep_pools;
