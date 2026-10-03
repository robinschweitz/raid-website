DROP TABLE IF EXISTS sessions;
DROP INDEX IF EXISTS idx_players_username;
ALTER TABLE players DROP COLUMN role;
ALTER TABLE players DROP COLUMN password_hash;
ALTER TABLE players DROP COLUMN username;
