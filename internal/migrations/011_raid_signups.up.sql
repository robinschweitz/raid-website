CREATE TABLE raid_signup_settings (
    raid_id INTEGER PRIMARY KEY REFERENCES raids(id),
    is_enabled BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE TABLE raid_signups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    raid_id INTEGER NOT NULL REFERENCES raids(id),
    player_id INTEGER NOT NULL REFERENCES players(id),
    character_id INTEGER NOT NULL REFERENCES characters(id),
    spec TEXT NOT NULL,
    is_withdrawn BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    UNIQUE (raid_id, character_id)
);
CREATE INDEX raid_signups_player ON raid_signups(raid_id, player_id);
