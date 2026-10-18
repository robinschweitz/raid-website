CREATE TABLE shadowmourne_queue_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    character_id INTEGER NOT NULL REFERENCES characters(id),
    queue_position INTEGER NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('QUEUED', 'COLLECTING', 'COMPLETED')),
    shards_collected INTEGER NOT NULL DEFAULT 0 CHECK (shards_collected >= 0),
    shard_target INTEGER NOT NULL CHECK (shard_target > 0),
    completed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE UNIQUE INDEX idx_shadowmourne_single_collector ON shadowmourne_queue_entries(status) WHERE status = 'COLLECTING';
CREATE UNIQUE INDEX idx_shadowmourne_queue_position ON shadowmourne_queue_entries(queue_position) WHERE status = 'QUEUED';
CREATE INDEX idx_shadowmourne_character ON shadowmourne_queue_entries(character_id);
