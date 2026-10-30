package raidcorrection

import (
	"context"

	"github.com/uptrace/bun"

	"raid-website/internal/attendance"
	"raid-website/internal/audit"
	"raid-website/internal/bosskill"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/lootcatalog"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/unclaimedloot"
)

// Repo owns the transaction boundary for cross-domain corrections.
type Repo struct {
	db         bun.IDB
	attendance *attendance.Repo
	loot       *loot.Repo
	raids      *raid.Repo
	players    *player.Repo
	characters *character.Repo
	points     *enhancepoints.Repo
	audit      *audit.Repo
	kills      *bosskill.Repo
	unclaimed  *unclaimedloot.Service
}

func NewRepo(db bun.IDB) *Repo {
	unclaimed := unclaimedloot.NewService(unclaimedloot.NewRepo(db), raid.NewRepo(db), lootcatalog.NewService(lootcatalog.NewRepo(db)))
	return &Repo{unclaimed: unclaimed, db: db, attendance: attendance.NewRepo(db), loot: loot.NewRepo(db), raids: raid.NewRepo(db), players: player.NewRepo(db), characters: character.NewRepo(db), points: enhancepoints.NewRepo(db), audit: audit.NewRepo(db), kills: bosskill.NewRepo(db)}
}

func (r *Repo) WithTx(tx bun.Tx) *Repo { return NewRepo(tx) }

func (r *Repo) quickAddRequest(ctx context.Context, raidID, actor int64, requestID string) (string, int64, error) {
	var row struct {
		Payload      string
		AttendanceID int64
	}
	err := r.db.NewSelect().TableExpr("raid_quick_add_requests").Column("payload", "attendance_id").Where("raid_id = ? AND actor_id = ? AND request_id = ?", raidID, actor, requestID).Scan(ctx, &row)
	return row.Payload, row.AttendanceID, err
}

func (r *Repo) saveQuickAddRequest(ctx context.Context, raidID, actor int64, requestID, payload string, attendanceID int64) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO raid_quick_add_requests (raid_id, actor_id, request_id, payload, attendance_id) VALUES (?, ?, ?, ?, ?)", raidID, actor, requestID, payload, attendanceID)
	return err
}

// deleteQuickAddRequests drops the idempotency records that point at a removed
// roster row; they only exist to replay a quick-add and reference the row by FK.
func (r *Repo) deleteQuickAddRequests(ctx context.Context, attendanceID int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM raid_quick_add_requests WHERE attendance_id = ?", attendanceID)
	return err
}

func (r *Repo) InTransaction(ctx context.Context, fn func(*Repo) error) error {
	if _, isBound := r.db.(bun.Tx); isBound {
		if err := r.loot.ReserveWriter(ctx); err != nil {
			return err
		}
		return fn(r)
	}
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		repo := r.WithTx(tx)
		if err := repo.loot.ReserveWriter(ctx); err != nil {
			return err
		}
		return fn(repo)
	})
}
