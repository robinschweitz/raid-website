package shadowmourne

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
)

var ErrNotFound = errors.New("shadowmourne queue entry not found")

type row struct {
	bun.BaseModel `bun:"table:shadowmourne_queue_entries"`

	ID              int64 `bun:"id,pk,autoincrement"`
	CharacterID     int64
	QueuePosition   int
	Status          string
	ShardsCollected int
	ShardTarget     int
	CompletedAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type entryRow struct {
	row
	CharacterName string
	PlayerID      int64
	PlayerName    string
}

func (r entryRow) entry() *Entry {
	return &Entry{
		ID: r.ID, CharacterID: r.CharacterID, CharacterName: r.CharacterName,
		PlayerID: r.PlayerID, PlayerName: r.PlayerName, QueuePosition: r.QueuePosition,
		Status: Status(r.Status), ShardsCollected: r.ShardsCollected, ShardTarget: r.ShardTarget,
		CompletedAt: r.CompletedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
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
		TableExpr("shadowmourne_queue_entries AS entries").
		ColumnExpr("entries.id, entries.character_id, entries.queue_position, entries.status, entries.shards_collected, entries.shard_target, entries.completed_at, entries.created_at, entries.updated_at, characters.name AS character_name, players.id AS player_id, players.name AS player_name").
		Join("JOIN characters ON characters.id = entries.character_id").
		Join("JOIN players ON players.id = characters.player_id")
}

func (r *Repo) List(ctx context.Context) ([]Entry, error) {
	var rows []entryRow
	err := r.entryQuery().
		OrderExpr("CASE entries.status WHEN 'COLLECTING' THEN 0 WHEN 'QUEUED' THEN 1 ELSE 2 END").
		OrderExpr("CASE WHEN entries.status = 'COMPLETED' THEN entries.completed_at END DESC").
		OrderExpr("entries.queue_position ASC, entries.id ASC").
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(rows))
	for _, item := range rows {
		entries = append(entries, *item.entry())
	}
	return entries, nil
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

func (r *Repo) Active(ctx context.Context) (*Entry, error) {
	var item entryRow
	err := r.entryQuery().Where("entries.status = ?", StatusCollecting).Scan(ctx, &item)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item.entry(), nil
}

func (r *Repo) NextQueued(ctx context.Context) (*Entry, error) {
	var item entryRow
	err := r.entryQuery().Where("entries.status = ?", StatusQueued).OrderExpr("entries.queue_position ASC, entries.id ASC").Limit(1).Scan(ctx, &item)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item.entry(), nil
}

func (r *Repo) Neighbor(ctx context.Context, entry *Entry, direction Direction) (*Entry, error) {
	query := r.entryQuery().Where("entries.status = ?", StatusQueued)
	if direction == MoveUp {
		query = query.Where("entries.queue_position < ?", entry.QueuePosition).OrderExpr("entries.queue_position DESC").Limit(1)
	} else {
		query = query.Where("entries.queue_position > ?", entry.QueuePosition).OrderExpr("entries.queue_position ASC").Limit(1)
	}
	var item entryRow
	err := query.Scan(ctx, &item)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item.entry(), nil
}

func (r *Repo) NextPosition(ctx context.Context) (int, error) {
	var position int
	err := r.db.NewSelect().TableExpr("shadowmourne_queue_entries").ColumnExpr("COALESCE(MAX(queue_position), 0) + 1").Scan(ctx, &position)
	return position, err
}

func (r *Repo) Create(ctx context.Context, entry *Entry) error {
	record := &row{
		CharacterID: entry.CharacterID, QueuePosition: entry.QueuePosition, Status: string(entry.Status),
		ShardsCollected: entry.ShardsCollected, ShardTarget: entry.ShardTarget, CreatedAt: entry.CreatedAt, UpdatedAt: entry.UpdatedAt,
	}
	_, err := r.db.NewInsert().Model(record).Exec(ctx)
	entry.ID = record.ID
	return err
}

func (r *Repo) SaveProgress(ctx context.Context, entry *Entry) error {
	_, err := r.db.NewUpdate().Table("shadowmourne_queue_entries").
		Set("shards_collected = ?", entry.ShardsCollected).
		Set("shard_target = ?", entry.ShardTarget).
		Set("updated_at = ?", entry.UpdatedAt).
		Where("id = ?", entry.ID).
		Exec(ctx)
	return err
}

func (r *Repo) Complete(ctx context.Context, entry *Entry) error {
	_, err := r.db.NewUpdate().Table("shadowmourne_queue_entries").
		Set("status = ?", StatusCompleted).
		Set("completed_at = ?", entry.CompletedAt).
		Set("updated_at = ?", entry.UpdatedAt).
		Where("id = ?", entry.ID).
		Exec(ctx)
	return err
}

func (r *Repo) Activate(ctx context.Context, entry *Entry) error {
	_, err := r.db.NewUpdate().Table("shadowmourne_queue_entries").
		Set("status = ?", StatusCollecting).
		Set("updated_at = ?", entry.UpdatedAt).
		Where("id = ?", entry.ID).
		Exec(ctx)
	return err
}

func (r *Repo) SwapQueuePositions(ctx context.Context, first, second *Entry, updatedAt time.Time) error {
	// A temporary negative position avoids the unique queue-position index while swapping.
	if _, err := r.db.NewUpdate().Table("shadowmourne_queue_entries").Set("queue_position = ?", -first.ID).Set("updated_at = ?", updatedAt).Where("id = ?", first.ID).Exec(ctx); err != nil {
		return err
	}
	if _, err := r.db.NewUpdate().Table("shadowmourne_queue_entries").Set("queue_position = ?", first.QueuePosition).Set("updated_at = ?", updatedAt).Where("id = ?", second.ID).Exec(ctx); err != nil {
		return err
	}
	_, err := r.db.NewUpdate().Table("shadowmourne_queue_entries").Set("queue_position = ?", second.QueuePosition).Set("updated_at = ?", updatedAt).Where("id = ?", first.ID).Exec(ctx)
	return err
}

func (r *Repo) Record(ctx context.Context, actorID int64, entityID int64, action string, before, after any) error {
	return audit.NewRepo(r.db).Record(ctx, actorID, "shadowmourne_queue_entry", entityID, action, before, after, "")
}
