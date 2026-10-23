-- Admin-set boss order per instance for boss pickers and loot lists (ADR 088).
-- Bosses without a row follow those with one, in catalog order.
CREATE TABLE loot_catalog_boss_order (
    boss_id TEXT PRIMARY KEY REFERENCES loot_catalog_bosses(id),
    position INTEGER NOT NULL CHECK (position >= 0),
    updated_by_player_id INTEGER NOT NULL REFERENCES players(id),
    updated_at TIMESTAMP NOT NULL
);
