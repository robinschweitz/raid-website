-- RaidSync addon lines identify one award per raid, whoever imports them (ADR 045).
CREATE UNIQUE INDEX idx_loot_raid_request ON loot_awards(raid_id, request_id) WHERE request_id LIKE 'raidsync-%';
