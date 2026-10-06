CREATE TABLE raid_quick_add_requests (
    raid_id INTEGER NOT NULL REFERENCES raids(id),
    actor_id INTEGER NOT NULL REFERENCES players(id),
    request_id TEXT NOT NULL,
    payload TEXT NOT NULL,
    attendance_id INTEGER NOT NULL REFERENCES raid_attendance(id),
    PRIMARY KEY (raid_id, actor_id, request_id)
);
