-- A push subscription belongs to the session that created it (FR-32.3,
-- ADR 103). When that session ends (logout, password or account change,
-- account merge), the subscription goes with it. Rows from before this
-- migration have no session and get no messages until the browser subscribes
-- again, which the web client does on the next visit.
ALTER TABLE push_subscriptions ADD COLUMN session_id INTEGER REFERENCES sessions(id) ON DELETE CASCADE;
CREATE INDEX idx_push_subscriptions_session ON push_subscriptions(session_id);
