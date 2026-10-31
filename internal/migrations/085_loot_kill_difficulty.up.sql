-- Empty identifies loot recorded before per-boss kill integration; no kills are inferred.
ALTER TABLE loot_awards ADD COLUMN difficulty TEXT NOT NULL DEFAULT '' CHECK (difficulty IN ('', 'NORMAL', 'HEROIC', 'HARDMODE'));
