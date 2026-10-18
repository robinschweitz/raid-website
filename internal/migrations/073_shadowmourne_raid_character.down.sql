CREATE TABLE shadowmourne_raid_assignments_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    raid_id INTEGER NOT NULL REFERENCES raids(id),
    task TEXT NOT NULL CHECK (task IN ('UNHOLY', 'BLOOD', 'FROST', 'SHARDS')),
    entry_id INTEGER REFERENCES shadowmourne_entries(id),
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    UNIQUE (raid_id, task)
);

-- Choices of characters without a list entry fall back to the suggestion.
INSERT INTO shadowmourne_raid_assignments_old (id, raid_id, task, entry_id, created_at, updated_at)
SELECT assignments.id, assignments.raid_id, assignments.task,
       (SELECT MAX(entries.id) FROM shadowmourne_entries AS entries WHERE entries.character_id = assignments.character_id),
       assignments.created_at, assignments.updated_at
FROM shadowmourne_raid_assignments AS assignments;

DROP TABLE shadowmourne_raid_assignments;
ALTER TABLE shadowmourne_raid_assignments_old RENAME TO shadowmourne_raid_assignments;
