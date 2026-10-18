ALTER TABLE raid_series ADD COLUMN plan_weeks_ahead INTEGER NOT NULL DEFAULT 0 CHECK (plan_weeks_ahead >= 0);
