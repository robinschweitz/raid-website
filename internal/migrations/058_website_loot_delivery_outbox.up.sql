CREATE TABLE website_loot_delivery_outbox (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    operation_id TEXT NOT NULL UNIQUE,
    source TEXT NOT NULL,
    sheet_id TEXT NOT NULL,
    raid_key TEXT NOT NULL,
    loot_award_id INTEGER NOT NULL REFERENCES loot_awards(id),
    import_id TEXT NOT NULL,
    operation_type TEXT NOT NULL CHECK (operation_type IN ('UPSERT', 'WITHDRAW')),
    payload_json TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('PENDING', 'SENT', 'FAILED', 'SUPERSEDED')),
    attempt_count INTEGER NOT NULL DEFAULT 0,
    last_attempt_at DATETIME,
    attempt_error TEXT NOT NULL DEFAULT '',
    created_by_player_id INTEGER NOT NULL REFERENCES players(id),
    created_at DATETIME NOT NULL,
    sent_at DATETIME,
    UNIQUE (source, sheet_id, loot_award_id, operation_id)
);
CREATE INDEX idx_website_loot_delivery_outbox_pending
ON website_loot_delivery_outbox(status, id);

CREATE TABLE website_loot_delivery_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    outbox_id INTEGER NOT NULL REFERENCES website_loot_delivery_outbox(id),
    event_type TEXT NOT NULL CHECK (event_type IN ('ENQUEUED', 'SENT', 'FAILED', 'SUPERSEDED')),
    detail TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL
);
CREATE INDEX idx_website_loot_delivery_events_outbox
ON website_loot_delivery_events(outbox_id, id);
CREATE TRIGGER website_loot_delivery_events_no_update BEFORE UPDATE ON website_loot_delivery_events
BEGIN SELECT RAISE(ABORT, 'website loot delivery events are append-only'); END;
CREATE TRIGGER website_loot_delivery_events_no_delete BEFORE DELETE ON website_loot_delivery_events
BEGIN SELECT RAISE(ABORT, 'website loot delivery events are append-only'); END;
