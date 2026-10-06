package raid

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/lootcatalog"
	"raid-website/internal/player"
)

type Repo struct {
	db bun.IDB
}

func NewRepo(db bun.IDB) *Repo {
	return &Repo{db: db}
}

func (r *Repo) WithTx(tx bun.Tx) *Repo {
	return &Repo{db: tx}
}

// Catalog binds catalog validation to the lifecycle transaction.
func (r *Repo) Catalog() *lootcatalog.Service {
	return lootcatalog.NewService(lootcatalog.NewRepo(r.db))
}

func (r *Repo) InTransaction(ctx context.Context, fn func(*Repo) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		repo := r.WithTx(tx)
		if err := repo.SerializeLifecycle(ctx); err != nil {
			return err
		}
		return fn(repo)
	})
}

// SerializeLifecycle acquires SQLite's writer reservation before any lifecycle reads.
func (r *Repo) SerializeLifecycle(ctx context.Context) error {
	_, err := r.db.NewUpdate().Table("raids").Set("id = id").Where("1 = 0").Exec(ctx)
	return err
}

func (r *Repo) Successor(ctx context.Context, id int64) (*Raid, error) {
	rd := new(Raid)
	err := r.db.NewSelect().Model(rd).Where("previous_raid_id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return rd, err
}

// FindOccurrence resolves a configured occurrence without changing manually planned data.
func (r *Repo) FindOccurrence(ctx context.Context, seriesID int64, scheduledStart time.Time) (*Raid, error) {
	rd := new(Raid)
	err := r.db.NewSelect().Model(rd).Where("series_id = ?", seriesID).Where("scheduled_start = ?", scheduledStart.UTC()).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return rd, nil
}

// LinkSuccessor changes only the linkage, preserving every field of a preplanned raid.
func (r *Repo) LinkSuccessor(ctx context.Context, id, previousRaidID int64) error {
	result, err := r.db.NewUpdate().Table("raids").Set("previous_raid_id = ?", previousRaidID).
		Where("id = ?", id).Where("previous_raid_id IS NULL OR previous_raid_id = ?", previousRaidID).Exec(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: raids.previous_raid_id") {
			return errors.Join(ErrOccurrenceConflict, err)
		}
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrOccurrenceConflict
	}
	return nil
}

func (r *Repo) RequireActor(ctx context.Context, actorID int64) error {
	_, err := player.NewRepo(r.db).Get(ctx, actorID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrActorNotFound
	}
	return err
}

func (r *Repo) Create(ctx context.Context, rd *Raid) error {
	_, err := r.db.NewInsert().Model(rd).Exec(ctx)
	if err != nil && (strings.Contains(err.Error(), "UNIQUE constraint failed: raids.series_id, raids.scheduled_start") || strings.Contains(err.Error(), "UNIQUE constraint failed: raids.previous_raid_id")) {
		return errors.Join(ErrOccurrenceConflict, err)
	}
	return err
}

func (r *Repo) Get(ctx context.Context, id int64) (*Raid, error) {
	rd := new(Raid)
	err := r.db.NewSelect().Model(rd).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.Join(ErrNotFound, err)
	}
	if err != nil {
		return nil, err
	}
	return rd, nil
}

// List always includes historical raids; the existing filter argument is no longer applied.
func (r *Repo) List(ctx context.Context, _ bool) ([]Raid, error) {
	var rs []Raid
	q := r.db.NewSelect().Model(&rs)
	err := q.Order("scheduled_start DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return rs, nil
}

func (r *Repo) Update(ctx context.Context, rd *Raid) error {
	_, err := r.db.NewUpdate().Model(rd).WherePK().Exec(ctx)
	if err != nil && (strings.Contains(err.Error(), "UNIQUE constraint failed: raids.series_id, raids.scheduled_start") || strings.Contains(err.Error(), "UNIQUE constraint failed: raids.previous_raid_id")) {
		return errors.Join(ErrOccurrenceConflict, err)
	}
	return err
}

func (r *Repo) ListBySeries(ctx context.Context, seriesID int64) ([]Raid, error) {
	var rs []Raid
	err := r.db.NewSelect().Model(&rs).Where("series_id = ?", seriesID).Order("scheduled_start ASC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return rs, nil
}

func (r *Repo) FindByName(ctx context.Context, name string) (*Raid, error) {
	rd := new(Raid)
	err := r.db.NewSelect().Model(rd).Where("name = ?", name).Order("id ASC").Limit(1).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return rd, nil
}
