CREATE TABLE raid_absences (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    raid_id INTEGER NOT NULL REFERENCES raids(id),
    player_id INTEGER NOT NULL REFERENCES players(id),
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL,
    UNIQUE (raid_id, player_id)
);
