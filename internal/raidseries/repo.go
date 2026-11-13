package raidseries

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/lootcatalog"
	"raid-website/internal/player"
)

type row struct {
	bun.BaseModel     `bun:"table:raid_series"`
	ID                int64 `bun:"id,pk,autoincrement"`
	CatalogInstanceID *string
	RaidSize          *int
	Name              string
	NameTemplate      string
	NextRaidNumber    int
	RaidLeaderID      int64
	Weekdays          string
	IntervalWeeks     int64
	StartTime         string
	Timezone          string
	PlanWeeksAhead    int
	// Written only by SetEP, so saving other fields keeps them (ADR 113).
	EPPoolID  *int64 `bun:"ep_pool_id"`
	EPAward   int    `bun:"ep_award"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Repo owns persistence for series configuration.
type Repo struct{ db bun.IDB }

func NewRepo(db bun.IDB) *Repo { return &Repo{db: db} }

// Catalog binds catalog validation to the configuration transaction.
func (r *Repo) Catalog() *lootcatalog.Service {
	return lootcatalog.NewService(lootcatalog.NewRepo(r.db))
}
func (r *Repo) WithTx(tx bun.Tx) *Repo { return NewRepo(tx) }

// Audit records a series change in the same transaction.
func (r *Repo) Audit(ctx context.Context, actorID, seriesID int64, action string, before, after *Series) error {
	var b any
	if before != nil {
		b = before
	}
	return audit.NewRepo(r.db).Record(ctx, actorID, "raid_series", seriesID, action, b, after, "")
}
func (r *Repo) InTransaction(ctx context.Context, fn func(*Repo) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Reserve the writer before reading configuration to avoid stale PATCH merges.
		if _, err := tx.NewUpdate().Table("raid_series").Set("id = id").Where("1 = 0").Exec(ctx); err != nil {
			return err
		}
		return fn(r.WithTx(tx))
	})
}

// SetEP stores the series' EP pool and award (FR-16.11).
func (r *Repo) SetEP(ctx context.Context, seriesID int64, poolID *int64, award int) error {
	_, err := r.db.NewUpdate().Table("raid_series").Set("ep_pool_id = ?", poolID).Set("ep_award = ?", award).Where("id = ?", seriesID).Exec(ctx)
	return err
}

// LoadEP reads the stored pool and award, including column defaults after an insert.
func (r *Repo) LoadEP(ctx context.Context, s *Series) error {
	return r.db.NewSelect().Table("raid_series").Column("ep_pool_id", "ep_award").Where("id = ?", s.ID).Scan(ctx, &s.EPPoolID, &s.EPAward)
}

// RequirePool rejects an unknown EP pool.
func (r *Repo) RequirePool(ctx context.Context, poolID int64) error {
	_, err := enhancepoints.NewRepo(r.db).GetPool(ctx, poolID)
	if errors.Is(err, enhancepoints.ErrPoolNotFound) {
		return ErrPoolNotFound
	}
	return err
}

func (r *Repo) RequireLeader(ctx context.Context, id int64) error {
	_, err := player.NewRepo(r.db).Get(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrLeaderNotFound
	}
	return err
}
func decode(rd row) (*Series, error) {
	s := &Series{ID: rd.ID, Name: rd.Name, NameTemplate: rd.NameTemplate, NextRaidNumber: rd.NextRaidNumber, RaidLeaderID: rd.RaidLeaderID, IntervalWeeks: rd.IntervalWeeks, StartTime: rd.StartTime, Timezone: rd.Timezone, PlanWeeksAhead: rd.PlanWeeksAhead, CreatedAt: rd.CreatedAt, UpdatedAt: rd.UpdatedAt}
	s.CatalogInstanceID, s.RaidSize = rd.CatalogInstanceID, rd.RaidSize
	s.EPPoolID, s.EPAward = rd.EPPoolID, rd.EPAward
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
	s, err := decode(rd)
	if err != nil {
		return nil, err
	}
	return s, r.loadLeaders(ctx, s)
}

// loadLeaders fills LeaderIDs: the first leader, then the others in order (ADR 112).
func (r *Repo) loadLeaders(ctx context.Context, s *Series) error {
	extra := []int64{}
	if err := r.db.NewSelect().Table("raid_series_leaders").Column("player_id").Where("series_id = ?", s.ID).Order("position ASC").Scan(ctx, &extra); err != nil {
		return err
	}
	s.LeaderIDs = append([]int64{s.RaidLeaderID}, extra...)
	return nil
}

// ReplaceExtraLeaders stores the series leaders after the first one.
func (r *Repo) ReplaceExtraLeaders(ctx context.Context, seriesID int64, playerIDs []int64) error {
	if _, err := r.db.NewDelete().Table("raid_series_leaders").Where("series_id = ?", seriesID).Exec(ctx); err != nil {
		return err
	}
	for position, playerID := range playerIDs {
		if _, err := r.db.ExecContext(ctx, "INSERT INTO raid_series_leaders (series_id, player_id, position) VALUES (?, ?, ?)", seriesID, playerID, position+1); err != nil {
			return err
		}
	}
	return nil
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
		if err := r.loadLeaders(ctx, s); err != nil {
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
	rd := &row{ID: s.ID, Name: s.Name, NameTemplate: s.NameTemplate, NextRaidNumber: s.NextRaidNumber, RaidLeaderID: s.RaidLeaderID, Weekdays: string(weekdays), IntervalWeeks: s.IntervalWeeks, StartTime: s.StartTime, Timezone: s.Timezone, PlanWeeksAhead: s.PlanWeeksAhead, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
	rd.CatalogInstanceID, rd.RaidSize = s.CatalogInstanceID, s.RaidSize
	if s.ID == 0 {
		_, err = r.db.NewInsert().Model(rd).ExcludeColumn("ep_pool_id", "ep_award").Exec(ctx)
		s.ID = rd.ID
	} else {
		_, err = r.db.NewUpdate().Model(rd).ExcludeColumn("ep_pool_id", "ep_award").WherePK().Exec(ctx)
	}
	return err
}
