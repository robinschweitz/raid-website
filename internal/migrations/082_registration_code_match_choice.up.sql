ALTER TABLE registration_codes ADD COLUMN match_choice TEXT NOT NULL DEFAULT '';
ALTER TABLE registration_codes ADD COLUMN match_player_id INTEGER;
