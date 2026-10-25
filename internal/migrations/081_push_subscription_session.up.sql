-- A push subscription belongs to the session that created it and ends with it
-- (FR-32.3, ADR 103). Opting in is a decision of one signed-in session; a new
-- session never inherits it. Rows from before this migration have no session
-- and are dropped, so those players turn notifications on again once.
DROP TABLE push_subscriptions;
CREATE TABLE push_subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    player_id INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    session_id INTEGER NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    endpoint TEXT NOT NULL,
    p256dh TEXT NOT NULL,
    auth TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX idx_push_subscriptions_endpoint ON push_subscriptions(endpoint);
CREATE INDEX idx_push_subscriptions_player ON push_subscriptions(player_id);
CREATE INDEX idx_push_subscriptions_session ON push_subscriptions(session_id);
