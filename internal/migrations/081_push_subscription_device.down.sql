DROP INDEX IF EXISTS idx_push_subscriptions_device;
ALTER TABLE push_subscriptions DROP COLUMN device_hash;
