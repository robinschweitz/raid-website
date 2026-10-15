CREATE TABLE raid_recruitment_templates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    series_id INTEGER REFERENCES raid_series(id),
    raid_id INTEGER REFERENCES raids(id),
    text TEXT NOT NULL,
    setup_json TEXT NOT NULL,
    updated_by INTEGER NOT NULL REFERENCES players(id),
    updated_at TIMESTAMP NOT NULL,
    CHECK ((series_id IS NULL) <> (raid_id IS NULL))
);
CREATE UNIQUE INDEX raid_recruitment_templates_series ON raid_recruitment_templates(series_id) WHERE series_id IS NOT NULL;
CREATE UNIQUE INDEX raid_recruitment_templates_raid ON raid_recruitment_templates(raid_id) WHERE raid_id IS NOT NULL;
