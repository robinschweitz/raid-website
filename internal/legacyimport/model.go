package legacyimport

import (
	"time"

	"github.com/uptrace/bun"
)

type Import struct {
	bun.BaseModel `bun:"table:legacy_imports"`

	ID             int64      `bun:"id,pk,autoincrement"`
	ActorID        int64      `bun:"actor_id,notnull"`
	Source         string     `bun:"source,notnull"`
	SourceURL      string     `bun:"source_url,notnull,default:''"`
	RawHash        string     `bun:"raw_hash,notnull,default:''"`
	DryRunReport   string     `bun:"dry_run_report,notnull,default:''"`
	Status         string     `bun:"status,notnull"`
	WarningsCount  int        `bun:"warnings_count,notnull,default:0"`
	ErrorsCount    int        `bun:"errors_count,notnull,default:0"`
	CreatedCount   int        `bun:"created_count,notnull,default:0"`
	UpdatedCount   int        `bun:"updated_count,notnull,default:0"`
	UnchangedCount int        `bun:"unchanged_count,notnull,default:0"`
	StartedAt      *time.Time `bun:"started_at,nullzero"`
	CompletedAt    *time.Time `bun:"completed_at,nullzero"`
	Notes          string     `bun:"notes,notnull,default:''"`
}
