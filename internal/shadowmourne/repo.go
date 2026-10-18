package shadowmourne

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
)

var ErrNotFound = errors.New("shadowmourne entry not found")

type row struct {
	bun.BaseModel `bun:"table:shadowmourne_entries"`

	ID              int64 `bun:"id,pk,autoincrement"`
	CharacterID     int64
	Priority        int
	Stage           string
	ShardsCollected int
	ShardTarget     int
	CompletedAt     *time.Time
	RemovedAt       *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type entryRow struct {
	row
	CharacterName  string
	CharacterClass string
	PlayerID       int64
	PlayerName     string
}

func (r entryRow) entry() *Entry {
	status := StatusActive
	if r.CompletedAt != nil {
		status = StatusCompleted
	}
	return &Entry{
		ID: r.ID, CharacterID: r.CharacterID, CharacterName: r.CharacterName, CharacterClass: r.CharacterClass,
		PlayerID: r.PlayerID, PlayerName: r.PlayerName, Priority: r.Priority, Status: status,
		Stage: Stage(r.Stage), ShardsCollected: r.ShardsCollected, ShardTarget: r.ShardTarget,
		CompletedAt: r.CompletedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

type assignmentRow struct {
	bun.BaseModel `bun:"table:shadowmourne_raid_assignments"`

	ID        int64 `bun:"id,pk,autoincrement"`
	RaidID    int64
	Task      string
	EntryID   *int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// RaidState is what the raid-task rules need to know about a raid.
type RaidState struct {
	Cancelled bool
	Archived  bool
}

type Repo struct {
	db bun.IDB
}

func NewRepo(db bun.IDB) *Repo {
	return &Repo{db: db}
}

func (r *Repo) WithTx(tx bun.Tx) *Repo {
	return &Repo{db: tx}
}

func (r *Repo) InTransaction(ctx context.Context, fn func(*Repo) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return fn(r.WithTx(tx))
	})
}

func (r *Repo) entryQuery() *bun.SelectQuery {
	return r.db.NewSelect().
		TableExpr("shadowmourne_entries AS entries").
		ColumnExpr("entries.id, entries.character_id, entries.priority, entries.stage, entries.shards_collected, entries.shard_target, entries.completed_at, entries.removed_at, entries.created_at, entries.updated_at").
		ColumnExpr("characters.name AS character_name, characters.class AS character_class, players.id AS player_id, players.name AS player_name").
		Join("JOIN characters ON characters.id = entries.character_id").
		Join("JOIN players ON players.id = characters.player_id").
		Where("entries.removed_at IS NULL")
}

func scanEntries(rows []entryRow) []Entry {
	entries := make([]Entry, 0, len(rows))
	for _, item := range rows {
		entries = append(entries, *item.entry())
	}
	return entries
}

// List returns active entries in priority order, then completed entries, newest first.
func (r *Repo) List(ctx context.Context) ([]Entry, error) {
	var rows []entryRow
	err := r.entryQuery().
		OrderExpr("CASE WHEN entries.completed_at IS NULL THEN 0 ELSE 1 END").
		OrderExpr("CASE WHEN entries.completed_at IS NULL THEN entries.priority END ASC").
		OrderExpr("entries.completed_at DESC, entries.id ASC").
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	return scanEntries(rows), nil
}

func (r *Repo) ListActive(ctx context.Context) ([]Entry, error) {
	var rows []entryRow
	err := r.entryQuery().Where("entries.completed_at IS NULL").OrderExpr("entries.priority ASC, entries.id ASC").Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	return scanEntries(rows), nil
}

// ListActiveInRaid returns active entries whose character is in the raid roster, in priority order.
func (r *Repo) ListActiveInRaid(ctx context.Context, raidID int64) ([]Entry, error) {
	var rows []entryRow
	err := r.entryQuery().
		Where("entries.completed_at IS NULL").
		Where("entries.character_id IN (SELECT character_id FROM raid_attendance WHERE raid_id = ?)", raidID).
		OrderExpr("entries.priority ASC, entries.id ASC").
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	return scanEntries(rows), nil
}

func (r *Repo) Get(ctx context.Context, id int64) (*Entry, error) {
	var item entryRow
	err := r.entryQuery().Where("entries.id = ?", id).Scan(ctx, &item)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return item.entry(), nil
}

func (r *Repo) HasActiveCharacter(ctx context.Context, characterID int64) (bool, error) {
	return r.db.NewSelect().TableExpr("shadowmourne_entries").
		Where("character_id = ? AND completed_at IS NULL AND removed_at IS NULL", characterID).
		Exists(ctx)
}

func (r *Repo) Create(ctx context.Context, entry *Entry) error {
	record := &row{
		CharacterID: entry.CharacterID, Priority: entry.Priority, Stage: string(entry.Stage),
		ShardsCollected: entry.ShardsCollected, ShardTarget: entry.ShardTarget, CreatedAt: entry.CreatedAt, UpdatedAt: entry.UpdatedAt,
	}
	_, err := r.db.NewInsert().Model(record).Exec(ctx)
	entry.ID = record.ID
	return err
}

func (r *Repo) SaveProgress(ctx context.Context, entry *Entry) error {
	_, err := r.db.NewUpdate().Table("shadowmourne_entries").
		Set("stage = ?", entry.Stage).
		Set("shards_collected = ?", entry.ShardsCollected).
		Set("shard_target = ?", entry.ShardTarget).
		Set("updated_at = ?", entry.UpdatedAt).
		Where("id = ?", entry.ID).
		Exec(ctx)
	return err
}

func (r *Repo) Complete(ctx context.Context, id int64, at time.Time) error {
	_, err := r.db.NewUpdate().Table("shadowmourne_entries").
		Set("completed_at = ?", at).
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (r *Repo) Remove(ctx context.Context, id int64, at time.Time) error {
	_, err := r.db.NewUpdate().Table("shadowmourne_entries").
		Set("removed_at = ?", at).
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (r *Repo) SetPriority(ctx context.Context, id int64, priority int, at time.Time) error {
	_, err := r.db.NewUpdate().Table("shadowmourne_entries").
		Set("priority = ?", priority).
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

// RaidState reports whether the raid exists and is cancelled or archived.
func (r *Repo) RaidState(ctx context.Context, raidID int64) (*RaidState, error) {
	var state struct {
		Status     string
		ArchivedAt *time.Time
	}
	err := r.db.NewSelect().TableExpr("raids").ColumnExpr("status, archived_at").Where("id = ?", raidID).Scan(ctx, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRaidMissing
	}
	if err != nil {
		return nil, err
	}
	return &RaidState{Cancelled: state.Status == "CANCELLED", Archived: state.ArchivedAt != nil}, nil
}

func (r *Repo) InRaidRoster(ctx context.Context, raidID, characterID int64) (bool, error) {
	return r.db.NewSelect().TableExpr("raid_attendance").Where("raid_id = ? AND character_id = ?", raidID, characterID).Exists(ctx)
}

// Assignments maps task to the assigned entry ID for one raid.
func (r *Repo) Assignments(ctx context.Context, raidID int64) (map[Stage]int64, error) {
	var rows []assignmentRow
	if err := r.db.NewSelect().Model(&rows).Where("raid_id = ?", raidID).Scan(ctx); err != nil {
		return nil, err
	}
	assigned := map[Stage]int64{}
	for _, item := range rows {
		if item.EntryID != nil {
			assigned[Stage(item.Task)] = *item.EntryID
		}
	}
	return assigned, nil
}

func (r *Repo) SaveAssignment(ctx context.Context, raidID int64, task Stage, entryID *int64, at time.Time) error {
	record := &assignmentRow{RaidID: raidID, Task: string(task), EntryID: entryID, CreatedAt: at, UpdatedAt: at}
	_, err := r.db.NewInsert().Model(record).
		On("CONFLICT (raid_id, task) DO UPDATE").
		Set("entry_id = EXCLUDED.entry_id").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

func (r *Repo) Record(ctx context.Context, actorID int64, entityType string, entityID int64, action string, before, after any) error {
	return audit.NewRepo(r.db).Record(ctx, actorID, entityType, entityID, action, before, after, "")
}
