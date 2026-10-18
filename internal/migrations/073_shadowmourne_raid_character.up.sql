-- Raid choices now name a roster character, so characters without a list entry can be chosen (ADR 087).
CREATE TABLE shadowmourne_raid_assignments_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    raid_id INTEGER NOT NULL REFERENCES raids(id),
    task TEXT NOT NULL CHECK (task IN ('UNHOLY', 'BLOOD', 'FROST', 'SHARDS')),
    character_id INTEGER REFERENCES characters(id),
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    UNIQUE (raid_id, task)
);

INSERT INTO shadowmourne_raid_assignments_new (id, raid_id, task, character_id, created_at, updated_at)
SELECT assignments.id, assignments.raid_id, assignments.task, entries.character_id, assignments.created_at, assignments.updated_at
FROM shadowmourne_raid_assignments AS assignments
LEFT JOIN shadowmourne_entries AS entries ON entries.id = assignments.entry_id;

DROP TABLE shadowmourne_raid_assignments;
ALTER TABLE shadowmourne_raid_assignments_new RENAME TO shadowmourne_raid_assignments;
