-- A push subscription belongs to a player on one device (FR-32.3, ADR 103).
-- device_hash is the SHA-256 of the HttpOnly raid_device cookie of the browser
-- that subscribed. It lets the server drop the subscription when another
-- player signs in on that device or the player signs out there. Rows from
-- before this migration have none until the browser checks in again.
ALTER TABLE push_subscriptions ADD COLUMN device_hash TEXT;
CREATE INDEX idx_push_subscriptions_device ON push_subscriptions(device_hash);
