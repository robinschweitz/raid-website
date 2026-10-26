package registration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"raid-website/internal/audit"
	"raid-website/internal/player"
	"strings"
	"time"

	"github.com/uptrace/bun"
)

type Repo struct {
	db bun.IDB
}

func NewRepo(db bun.IDB) *Repo { return &Repo{db: db} }

func (r *Repo) WithTx(tx bun.Tx) *Repo { return &Repo{db: tx} }

func (r *Repo) CreateRequest(ctx context.Context, req *Request) error {
	_, err := r.db.NewInsert().Model(req).Exec(ctx)
	if err != nil && strings.Contains(err.Error(), "active_registration_request_exists") {
		return ErrRequestExists
	}
	return err
}

func (r *Repo) FindBlockingRequest(ctx context.Context, source, profileID string) (*Request, error) {
	req := new(Request)
	err := r.db.NewSelect().Model(req).
		Where("external_source = ?", source).
		Where("external_profile_id = ?", profileID).
		Where("status IN (?, ?, ?)", StatusPending, StatusApproved, StatusConsumed).
		Order("created_at DESC", "id DESC").
		Limit(1).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return req, err
}

func (r *Repo) UpdateRequest(ctx context.Context, req *Request) error {
	_, err := r.db.NewUpdate().Model(req).WherePK().Exec(ctx)
	return err
}

func (r *Repo) GetRequest(ctx context.Context, id int64) (*Request, error) {
	req := new(Request)
	err := r.db.NewSelect().Model(req).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return req, nil
}

func (r *Repo) ListRequests(ctx context.Context, statuses []string) ([]Request, error) {
	var rows []Request
	query := r.db.NewSelect().Model(&rows).Order("created_at DESC", "id DESC")
	if len(statuses) > 0 {
		query.Where("status IN (?)", bun.List(statuses))
	}
	err := query.Scan(ctx)
	return rows, err
}

// ListRedemptions returns every used code with its request, oldest first.
func (r *Repo) ListRedemptions(ctx context.Context) ([]Redemption, error) {
	var rows []Redemption
	err := r.db.NewSelect().
		TableExpr("registration_codes AS c").
		Join("JOIN registration_requests AS r ON r.id = c.request_id").
		ColumnExpr("c.request_id, r.purpose, r.character_name, c.used_by_player_id, c.used_at").
		Where("c.used_at IS NOT NULL").
		Where("c.used_by_player_id IS NOT NULL").
		OrderExpr("c.used_at ASC, c.id ASC").
		Scan(ctx, &rows)
	return rows, err
}

func (r *Repo) CreateCode(ctx context.Context, code *Code) error {
	_, err := r.db.NewInsert().Model(code).Exec(ctx)
	return err
}

func (r *Repo) UpdateCode(ctx context.Context, code *Code) error {
	_, err := r.db.NewUpdate().Model(code).WherePK().Exec(ctx)
	return err
}

func (r *Repo) GetCode(ctx context.Context, id int64) (*Code, error) {
	code := new(Code)
	err := r.db.NewSelect().Model(code).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return code, nil
}

func (r *Repo) FindActiveCodeByHash(ctx context.Context, hash string, now time.Time) (*CodeWithRequest, error) {
	row := new(CodeWithRequest)
	err := r.db.NewSelect().Model(row).Relation("Request").Where("code_hash = ?", hash).Where("used_at IS NULL").Where("revoked_at IS NULL").Where("expires_at > ?", now).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (r *Repo) ListCodesByRequest(ctx context.Context, requestID int64) ([]Code, error) {
	var rows []Code
	err := r.db.NewSelect().Model(&rows).Where("request_id = ?", requestID).Order("created_at DESC", "id DESC").Scan(ctx)
	return rows, err
}

func (r *Repo) inResetTransaction(ctx context.Context, fn func(*Repo) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE players SET id = id WHERE id = -1"); err != nil {
			return err
		}
		return fn(NewRepo(tx))
	})
}
func (r *Repo) resetPlayer(ctx context.Context, id int64) (*player.Player, error) {
	return player.NewRepo(r.db).Get(ctx, id)
}
func (r *Repo) revokeResetCodes(ctx context.Context, playerID int64, now time.Time) error {
	_, err := r.db.NewUpdate().Model((*ResetCode)(nil)).Set("revoked_at = ?", now).Where("player_id = ? AND used_at IS NULL AND revoked_at IS NULL", playerID).Exec(ctx)
	return err
}
func (r *Repo) createResetCode(ctx context.Context, row *ResetCode) error {
	_, err := r.db.NewInsert().Model(row).Exec(ctx)
	return err
}
func (r *Repo) auditReset(ctx context.Context, actorID, playerID int64, action string, details map[string]any, reason string) error {
	return audit.NewRepo(r.db).Record(ctx, actorID, "player", playerID, action, nil, details, reason)
}

func (r *Repo) operatorResetPlayer(ctx context.Context, name string) (*player.Player, error) {
	var rows []player.Player
	if err := r.db.NewSelect().Model(&rows).Where("name = ? OR username = ?", name, strings.ToLower(strings.TrimSpace(name))).Scan(ctx); err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("player name is unknown or ambiguous; use --player-id")
	}
	return &rows[0], nil
}
