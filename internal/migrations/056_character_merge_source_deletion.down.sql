ALTER TABLE character_merge_aliases RENAME TO character_merge_aliases_old;
CREATE TABLE character_merge_aliases (
  source_name TEXT NOT NULL PRIMARY KEY,
  target_character_id INTEGER NOT NULL REFERENCES characters(id),
  source_character_id INTEGER NOT NULL UNIQUE REFERENCES characters(id),
  created_at DATETIME NOT NULL
);
INSERT INTO character_merge_aliases (source_name, target_character_id, source_character_id, created_at)
SELECT source_name, target_character_id, source_character_id, created_at FROM character_merge_aliases_old
WHERE EXISTS (SELECT 1 FROM characters WHERE characters.id = character_merge_aliases_old.source_character_id);
DROP TABLE character_merge_aliases_old;
CREATE INDEX idx_character_merge_aliases_target ON character_merge_aliases(target_character_id);
