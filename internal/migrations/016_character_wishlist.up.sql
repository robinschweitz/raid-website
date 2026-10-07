CREATE TABLE character_wishlist (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  player_id INTEGER NOT NULL,
  character_id INTEGER NOT NULL,
  item_id TEXT NOT NULL,
  item_name TEXT NOT NULL,
  priority INTEGER NOT NULL DEFAULT 0,
  notes TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (player_id) REFERENCES players(id) ON DELETE RESTRICT,
  FOREIGN KEY (character_id) REFERENCES characters(id) ON DELETE RESTRICT,
  UNIQUE (character_id, item_id)
);
CREATE INDEX idx_wishlist_char ON character_wishlist(character_id);
CREATE INDEX idx_wishlist_item ON character_wishlist(item_id);
