DROP TABLE IF EXISTS poll_vote_choices;
DROP TABLE IF EXISTS poll_options;
ALTER TABLE polls DROP COLUMN allow_multiple;
ALTER TABLE polls DROP COLUMN poll_type;
