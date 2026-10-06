package raidseries

import "time"

// Series defines an explicitly configured local-wall-time raid schedule.
type Series struct {
	ID                int64     `json:"id"`
	CatalogInstanceID *string   `json:"catalog_instance_id"`
	RaidSize          *int      `json:"raid_size"`
	Name              string    `json:"name"`
	NameTemplate      string    `json:"name_template"`
	NextRaidNumber    int       `json:"next_raid_number"`
	RaidLeaderID      int64     `json:"raid_leader_id"`
	Weekdays          []int     `json:"weekdays"`
	IntervalWeeks     int64     `json:"interval_weeks"`
	StartTime         string    `json:"start_time"`
	Timezone          string    `json:"timezone"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
