DROP INDEX IF EXISTS idx_push_subscriptions_session;
CREATE TABLE push_subscriptions_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    player_id INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    endpoint TEXT NOT NULL,
    p256dh TEXT NOT NULL,
    auth TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL
);
INSERT INTO push_subscriptions_old (id, player_id, endpoint, p256dh, auth, created_at)
    SELECT id, player_id, endpoint, p256dh, auth, created_at FROM push_subscriptions;
DROP TABLE push_subscriptions;
ALTER TABLE push_subscriptions_old RENAME TO push_subscriptions;
CREATE UNIQUE INDEX idx_push_subscriptions_endpoint ON push_subscriptions(endpoint);
CREATE INDEX idx_push_subscriptions_player ON push_subscriptions(player_id);
