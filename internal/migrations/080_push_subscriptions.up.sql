-- Web Push subscriptions of signed-in players (FR-32). One row per browser;
-- the endpoint is unique because a browser has exactly one subscription. The
-- rows go with the player (merge source, FR-5.4).
CREATE TABLE push_subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    player_id INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    endpoint TEXT NOT NULL,
    p256dh TEXT NOT NULL,
    auth TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX idx_push_subscriptions_endpoint ON push_subscriptions(endpoint);
CREATE INDEX idx_push_subscriptions_player ON push_subscriptions(player_id);
