CREATE TABLE shadowmourne_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    character_id INTEGER NOT NULL REFERENCES characters(id),
    priority INTEGER NOT NULL,
    stage TEXT NOT NULL CHECK (stage IN ('PREQUEST', 'SOULS', 'UNHOLY', 'BLOOD', 'FROST', 'SHARDS', 'FINAL')),
    shards_collected INTEGER NOT NULL DEFAULT 0 CHECK (shards_collected >= 0),
    shard_target INTEGER NOT NULL CHECK (shard_target > 0),
    completed_at TIMESTAMP,
    removed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

-- A character can be on the list only once until it finished or was removed.
CREATE UNIQUE INDEX idx_shadowmourne_active_character ON shadowmourne_entries(character_id) WHERE completed_at IS NULL AND removed_at IS NULL;
CREATE INDEX idx_shadowmourne_priority ON shadowmourne_entries(priority);

-- entry_id NULL means the raid uses the suggestion from the list.
CREATE TABLE shadowmourne_raid_assignments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    raid_id INTEGER NOT NULL REFERENCES raids(id),
    task TEXT NOT NULL CHECK (task IN ('UNHOLY', 'BLOOD', 'FROST', 'SHARDS')),
    entry_id INTEGER REFERENCES shadowmourne_entries(id),
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    UNIQUE (raid_id, task)
);
