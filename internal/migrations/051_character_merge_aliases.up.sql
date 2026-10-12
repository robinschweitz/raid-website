CREATE TABLE character_merge_aliases (
 source_name TEXT NOT NULL PRIMARY KEY,
 target_character_id INTEGER NOT NULL REFERENCES characters(id),
 source_character_id INTEGER NOT NULL UNIQUE REFERENCES characters(id),
 created_at DATETIME NOT NULL
);
CREATE INDEX idx_character_merge_aliases_target ON character_merge_aliases(target_character_id);
