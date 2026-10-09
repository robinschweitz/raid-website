ALTER TABLE polls ADD COLUMN poll_type TEXT NOT NULL DEFAULT 'TEXT';
ALTER TABLE polls ADD COLUMN allow_multiple BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS poll_options (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  poll_id INTEGER NOT NULL,
  label TEXT NOT NULL,
  sort_order INTEGER NOT NULL,
  FOREIGN KEY (poll_id) REFERENCES polls(id),
  UNIQUE (poll_id, label)
);

CREATE TABLE IF NOT EXISTS poll_vote_choices (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  vote_id INTEGER NOT NULL,
  option_id INTEGER NOT NULL,
  label TEXT NOT NULL,
  sort_order INTEGER NOT NULL,
  FOREIGN KEY (vote_id) REFERENCES poll_votes(id),
  FOREIGN KEY (option_id) REFERENCES poll_options(id),
  UNIQUE (vote_id, option_id)
);
