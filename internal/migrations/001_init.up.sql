CREATE TABLE IF NOT EXISTS guild_ranks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  sort_order INTEGER NOT NULL DEFAULT 0,
  active INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS raid_groups (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  color TEXT,
  sort_order INTEGER NOT NULL DEFAULT 0,
  active INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS players (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  guild_rank_id INTEGER,
  active INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (guild_rank_id) REFERENCES guild_ranks(id)
);

CREATE TABLE IF NOT EXISTS characters (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  player_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  former_names TEXT,
  name_changed_at DATETIME,
  class TEXT NOT NULL,
  main_spec TEXT NOT NULL,
  second_spec TEXT NOT NULL,
  is_main INTEGER NOT NULL DEFAULT 0,
  active INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (player_id) REFERENCES players(id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_characters_name ON characters(name);

CREATE TABLE IF NOT EXISTS character_name_history (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  character_id INTEGER NOT NULL,
  old_name TEXT NOT NULL,
  new_name TEXT NOT NULL,
  actor_id INTEGER NOT NULL,
  reason TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  FOREIGN KEY (character_id) REFERENCES characters(id),
  FOREIGN KEY (actor_id) REFERENCES players(id)
);

CREATE TABLE IF NOT EXISTS raids (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  raid_leader_id INTEGER NOT NULL,
  status TEXT NOT NULL,
  scheduled_start DATETIME,
  scheduled_end DATETIME,
  started_at DATETIME,
  completed_at DATETIME,
  cancelled_at DATETIME,
  archived_at DATETIME,
  notes TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (raid_leader_id) REFERENCES players(id)
);
CREATE INDEX IF NOT EXISTS idx_raids_status ON raids(status);
CREATE INDEX IF NOT EXISTS idx_raids_archived_at ON raids(archived_at);

CREATE TABLE IF NOT EXISTS raid_attendance (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  raid_id INTEGER NOT NULL,
  player_id INTEGER NOT NULL,
  character_id INTEGER NOT NULL,
  role TEXT NOT NULL,
  is_substitute INTEGER NOT NULL DEFAULT 0,
  is_bench INTEGER NOT NULL DEFAULT 0,
  late_join INTEGER NOT NULL DEFAULT 0,
  early_leave INTEGER NOT NULL DEFAULT 0,
  notes TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (raid_id) REFERENCES raids(id) ON DELETE RESTRICT,
  FOREIGN KEY (player_id) REFERENCES players(id),
  FOREIGN KEY (character_id) REFERENCES characters(id),
  UNIQUE (raid_id, player_id, character_id)
);
CREATE INDEX IF NOT EXISTS idx_att_raid ON raid_attendance(raid_id);
CREATE INDEX IF NOT EXISTS idx_att_player ON raid_attendance(player_id);

CREATE TABLE IF NOT EXISTS loot_awards (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  raid_id INTEGER NOT NULL,
  boss TEXT NOT NULL,
  item_id TEXT NOT NULL,
  item_name TEXT NOT NULL,
  awarded_to_player_id INTEGER NOT NULL,
  awarded_to_character_id INTEGER,
  awarded_by_player_id INTEGER NOT NULL,
  source TEXT NOT NULL,
  is_main_spec INTEGER NOT NULL DEFAULT 0,
  item_lock_id INTEGER,
  notes TEXT NOT NULL DEFAULT '',
  awarded_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (raid_id) REFERENCES raids(id) ON DELETE RESTRICT,
  FOREIGN KEY (awarded_to_player_id) REFERENCES players(id),
  FOREIGN KEY (awarded_to_character_id) REFERENCES characters(id),
  FOREIGN KEY (awarded_by_player_id) REFERENCES players(id),
  FOREIGN KEY (item_lock_id) REFERENCES item_locks(id)
);

CREATE TABLE IF NOT EXISTS enhance_point_transactions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  player_id INTEGER NOT NULL,
  raid_id INTEGER,
  amount INTEGER NOT NULL,
  type TEXT NOT NULL,
  reason TEXT NOT NULL,
  ref_type TEXT NOT NULL DEFAULT '',
  ref_id TEXT NOT NULL DEFAULT '',
  actor_id INTEGER NOT NULL,
  created_at DATETIME NOT NULL,
  FOREIGN KEY (player_id) REFERENCES players(id),
  FOREIGN KEY (raid_id) REFERENCES raids(id)
);
CREATE INDEX IF NOT EXISTS idx_ep_player_created ON enhance_point_transactions(player_id, created_at);
CREATE INDEX IF NOT EXISTS idx_ep_raid ON enhance_point_transactions(raid_id);

CREATE TABLE IF NOT EXISTS item_locks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  player_id INTEGER NOT NULL,
  raid_id INTEGER NOT NULL,
  item_id TEXT NOT NULL DEFAULT '',
  item_name TEXT NOT NULL DEFAULT '',
  boss TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  locked_at DATETIME NOT NULL,
  used_at DATETIME,
  expired_at DATETIME,
  cancelled_at DATETIME,
  actor_id INTEGER NOT NULL,
  notes TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (player_id) REFERENCES players(id),
  FOREIGN KEY (raid_id) REFERENCES raids(id)
);
CREATE INDEX IF NOT EXISTS idx_locks_player_status ON item_locks(player_id, status);
CREATE INDEX IF NOT EXISTS idx_locks_raid_status ON item_locks(raid_id, status);

CREATE TABLE IF NOT EXISTS polls (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  closes_at DATETIME,
  created_by INTEGER NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (created_by) REFERENCES players(id)
);

CREATE TABLE IF NOT EXISTS poll_votes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  poll_id INTEGER NOT NULL,
  player_id INTEGER NOT NULL,
  choice TEXT NOT NULL,
  voted_at DATETIME NOT NULL,
  FOREIGN KEY (poll_id) REFERENCES polls(id),
  FOREIGN KEY (player_id) REFERENCES players(id),
  UNIQUE (poll_id, player_id)
);

CREATE TABLE IF NOT EXISTS legacy_imports (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  actor_id INTEGER NOT NULL,
  source TEXT NOT NULL,
  source_url TEXT NOT NULL DEFAULT '',
  raw_hash TEXT NOT NULL DEFAULT '',
  dry_run_report TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  warnings_count INTEGER NOT NULL DEFAULT 0,
  errors_count INTEGER NOT NULL DEFAULT 0,
  created_count INTEGER NOT NULL DEFAULT 0,
  updated_count INTEGER NOT NULL DEFAULT 0,
  unchanged_count INTEGER NOT NULL DEFAULT 0,
  started_at DATETIME,
  completed_at DATETIME,
  notes TEXT NOT NULL DEFAULT '',
  FOREIGN KEY (actor_id) REFERENCES players(id)
);

CREATE TABLE IF NOT EXISTS audit_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  actor_id INTEGER,
  entity_type TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  action TEXT NOT NULL,
  before_json TEXT,
  after_json TEXT,
  reason TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_entity_created ON audit_log(entity_type, entity_id, created_at);
CREATE INDEX IF NOT EXISTS idx_audit_actor_created ON audit_log(actor_id, created_at);
