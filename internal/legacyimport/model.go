package legacyimport

import "time"

type Import struct {
	ID             int64
	ActorID        int64
	Source         string
	SourceURL      string
	RawHash        string
	DryRunReport   string
	Status         string
	WarningsCount  int
	ErrorsCount    int
	CreatedCount   int
	UpdatedCount   int
	UnchangedCount int
	StartedAt      *time.Time
	CompletedAt    *time.Time
	Notes          string
}
