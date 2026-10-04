package raidcorrection

import (
	"context"

	"github.com/uptrace/bun"

	"raid-website/internal/attendance"
	"raid-website/internal/audit"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/tokens"
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
	tokens     *tokens.Repo
	audit      *audit.Repo
}

func NewRepo(db bun.IDB) *Repo {
	return &Repo{db: db, attendance: attendance.NewRepo(db), loot: loot.NewRepo(db), raids: raid.NewRepo(db), players: player.NewRepo(db), characters: character.NewRepo(db), points: enhancepoints.NewRepo(db), tokens: tokens.NewRepo(db), audit: audit.NewRepo(db)}
}

func (r *Repo) WithTx(tx bun.Tx) *Repo { return NewRepo(tx) }

func (r *Repo) InTransaction(ctx context.Context, fn func(*Repo) error) error {
	if _, isBound := r.db.(bun.Tx); isBound {
		return fn(r)
	}
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error { return fn(r.WithTx(tx)) })
}

func (r *Repo) tokenNets(ctx context.Context, awardID int64) (map[int64]int, error) {
	var rows []struct {
		PlayerID int64
		Amount   int
	}
	err := r.db.NewSelect().TableExpr("token_transactions").Column("player_id").ColumnExpr("SUM(amount) AS amount").Where("loot_award_id = ?", awardID).Group("player_id").Scan(ctx, &rows)
	nets := make(map[int64]int)
	for _, row := range rows {
		nets[row.PlayerID] = row.Amount
	}
	return nets, err
}
