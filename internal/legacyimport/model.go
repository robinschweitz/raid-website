package legacyimport

import (
	"time"

	"github.com/uptrace/bun"
)

type Import struct {
	bun.BaseModel `bun:"table:legacy_imports"`

	ID             int64      `bun:"id,pk,autoincrement" json:"id"`
	ActorID        int64      `bun:"actor_id,notnull" json:"actor_id"`
	Source         string     `bun:"source,notnull" json:"source"`
	SourceURL      string     `bun:"source_url,notnull,default:''" json:"source_url"`
	RawHash        string     `bun:"raw_hash,notnull,default:''" json:"raw_hash"`
	DryRunReport   string     `bun:"dry_run_report,notnull,default:''" json:"dry_run_report"`
	Status         string     `bun:"status,notnull" json:"status"`
	WarningsCount  int        `bun:"warnings_count,notnull,default:0" json:"warnings_count"`
	ErrorsCount    int        `bun:"errors_count,notnull,default:0" json:"errors_count"`
	CreatedCount   int        `bun:"created_count,notnull,default:0" json:"created_count"`
	UpdatedCount   int        `bun:"updated_count,notnull,default:0" json:"updated_count"`
	UnchangedCount int        `bun:"unchanged_count,notnull,default:0" json:"unchanged_count"`
	StartedAt      *time.Time `bun:"started_at,nullzero" json:"started_at"`
	CompletedAt    *time.Time `bun:"completed_at,nullzero" json:"completed_at"`
	Notes          string     `bun:"notes,notnull,default:''" json:"notes"`
}
