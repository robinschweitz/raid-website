package raidseries

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/uptrace/bun"
	"raid-website/internal/lootcatalog"
	"raid-website/internal/player"
)

type row struct {
	bun.BaseModel     `bun:"table:raid_series"`
	ID                int64 `bun:"id,pk,autoincrement"`
	CatalogInstanceID *string
	RaidSize          *int
	Name              string
	RaidLeaderID      int64
	Weekdays          string
	IntervalWeeks     int64
	StartTime         string
	Timezone          string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Repo owns persistence for series configuration.
type Repo struct{ db bun.IDB }

func NewRepo(db bun.IDB) *Repo { return &Repo{db: db} }

// Catalog binds catalog validation to the configuration transaction.
func (r *Repo) Catalog() *lootcatalog.Service {
	return lootcatalog.NewService(lootcatalog.NewRepo(r.db))
}
func (r *Repo) WithTx(tx bun.Tx) *Repo { return NewRepo(tx) }
func (r *Repo) InTransaction(ctx context.Context, fn func(*Repo) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Reserve the writer before reading configuration to avoid stale PATCH merges.
		if _, err := tx.NewUpdate().Table("raid_series").Set("id = id").Where("1 = 0").Exec(ctx); err != nil {
			return err
		}
		return fn(r.WithTx(tx))
	})
}
func (r *Repo) RequireLeader(ctx context.Context, id int64) error {
	_, err := player.NewRepo(r.db).Get(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrLeaderNotFound
	}
	return err
}
func decode(rd row) (*Series, error) {
	s := &Series{ID: rd.ID, Name: rd.Name, RaidLeaderID: rd.RaidLeaderID, IntervalWeeks: rd.IntervalWeeks, StartTime: rd.StartTime, Timezone: rd.Timezone, CreatedAt: rd.CreatedAt, UpdatedAt: rd.UpdatedAt}
	s.CatalogInstanceID, s.RaidSize = rd.CatalogInstanceID, rd.RaidSize
	if err := json.Unmarshal([]byte(rd.Weekdays), &s.Weekdays); err != nil {
		return nil, errors.Join(ErrInvalid, err)
	}
	return s, nil
}
func (r *Repo) Get(ctx context.Context, id int64) (*Series, error) {
	var rd row
	if err := r.db.NewSelect().Model(&rd).Where("id = ?", id).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return decode(rd)
}
func (r *Repo) List(ctx context.Context) ([]Series, error) {
	var rows []row
	if err := r.db.NewSelect().Model(&rows).Order("id ASC").Scan(ctx); err != nil {
		return nil, err
	}
	result := make([]Series, 0, len(rows))
	for _, rd := range rows {
		s, err := decode(rd)
		if err != nil {
			return nil, err
		}
		result = append(result, *s)
	}
	return result, nil
}
func (r *Repo) Save(ctx context.Context, s *Series) error {
	weekdays, err := json.Marshal(s.Weekdays)
	if err != nil {
		return err
	}
	rd := &row{ID: s.ID, Name: s.Name, RaidLeaderID: s.RaidLeaderID, Weekdays: string(weekdays), IntervalWeeks: s.IntervalWeeks, StartTime: s.StartTime, Timezone: s.Timezone, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
	rd.CatalogInstanceID, rd.RaidSize = s.CatalogInstanceID, s.RaidSize
	if s.ID == 0 {
		_, err = r.db.NewInsert().Model(rd).Exec(ctx)
		s.ID = rd.ID
	} else {
		_, err = r.db.NewUpdate().Model(rd).WherePK().Exec(ctx)
	}
	return err
}
