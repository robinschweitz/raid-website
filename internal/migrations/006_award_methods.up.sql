ALTER TABLE loot_awards ADD COLUMN award_method TEXT NOT NULL DEFAULT 'SECOND' CHECK (award_method IN ('MAIN', 'SECOND', 'ENHANCED', 'BOE'));
ALTER TABLE loot_awards ADD COLUMN enhanced_cost INTEGER NOT NULL DEFAULT 0 CHECK (enhanced_cost BETWEEN 0 AND 50);
UPDATE loot_awards SET award_method = CASE WHEN is_main_spec THEN 'MAIN' ELSE 'SECOND' END;
ALTER TABLE loot_awards ADD COLUMN request_id TEXT;
ALTER TABLE loot_awards ADD COLUMN request_hash TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX idx_loot_request ON loot_awards(awarded_by_player_id, request_id) WHERE request_id IS NOT NULL;
ALTER TABLE enhance_point_transactions ADD COLUMN loot_award_id INTEGER REFERENCES loot_awards(id);
CREATE INDEX idx_ep_loot_award ON enhance_point_transactions(loot_award_id);
