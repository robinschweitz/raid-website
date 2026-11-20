package raidpermission

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"
	"raid-website/internal/audit"
	"raid-website/internal/validation"
)

var ErrForbidden = errors.New("raid permission denied")
var ErrInvalid = errors.New("invalid raid permission")

type Repo struct{ DB *bun.DB }

func (r Repo) RaidSeries(ctx context.Context, raid int64) (int64, error) {
	var id int64
	err := r.DB.QueryRowContext(ctx, "SELECT COALESCE(series_id,0) FROM raids WHERE id=?", raid).Scan(&id)
	return id, err
}

type Staff struct {
	ID   int64           `json:"id"`
	Name string          `json:"name"`
	Role validation.Role `json:"role"`
}

func (r Repo) Team(ctx context.Context) ([]Staff, error) {
	out := []Staff{}
	err := r.DB.NewSelect().Table("players").Column("id", "name", "role").Where("active = 1 AND role IN ('RAIDASSISTENT','RAIDLEADER')").Order("name ASC", "id ASC").Scan(ctx, &out)
	return out, err
}

func policy(ctx context.Context, db bun.IDB, id int64) (*Policy, error) {
	var p Policy
	err := db.QueryRowContext(ctx, "SELECT allowed, assigned_only FROM raid_permission_policies WHERE player_id=?", id).Scan(&p.Allowed, &p.AssignedOnly)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &p, err
}
func (r Repo) Policy(ctx context.Context, id int64) (*Policy, error) { return policy(ctx, r.DB, id) }
func staff(ctx context.Context, db bun.IDB, id int64) (validation.Role, error) {
	var role validation.Role
	err := db.QueryRowContext(ctx, "SELECT role FROM players WHERE id=? AND active=1", id).Scan(&role)
	return role, err
}
func leader(ctx context.Context, db bun.IDB, id, raid int64) (bool, error) {
	var yes bool
	err := db.QueryRowContext(ctx, "SELECT raid_leader_id=? OR EXISTS(SELECT 1 FROM raid_leaders WHERE raid_id=raids.id AND player_id=?) FROM raids WHERE id=?", id, id, raid).Scan(&yes)
	return yes, err
}
func (r Repo) SeriesLeader(ctx context.Context, id, series int64) (bool, error) {
	var yes bool
	err := r.DB.QueryRowContext(ctx, "SELECT raid_leader_id=? OR EXISTS(SELECT 1 FROM raid_series_leaders WHERE series_id=raid_series.id AND player_id=?) FROM raid_series WHERE id=?", id, id, series).Scan(&yes)
	return yes, err
}
func (r Repo) HasAssignedPolicies(ctx context.Context) (bool, error) {
	var yes bool
	err := r.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM raid_permission_policies WHERE assigned_only=1)").Scan(&yes)
	return yes, err
}
func (r Repo) UnclaimedRaid(ctx context.Context, id int64) (int64, error) {
	var raid int64
	err := r.DB.QueryRowContext(ctx, "SELECT raid_id FROM unclaimed_loot WHERE id=?", id).Scan(&raid)
	return raid, err
}
func (r Repo) Effective(ctx context.Context, id, raid int64, role validation.Role) (Effective, error) {
	if role == validation.RoleAdmin || !role.AtLeast(validation.RoleRaidassistent) {
		return Base(role, nil, false), nil
	}
	p, err := r.Policy(ctx, id)
	if err != nil {
		return Effective{}, err
	}
	if p == nil || !p.AssignedOnly {
		return Base(role, p, false), nil
	}
	own, err := leader(ctx, r.DB, id, raid)
	if err != nil {
		return Effective{}, err
	}
	base := Base(role, p, own)
	if own {
		return base, nil
	}
	var from int64
	var actions Actions
	err = r.DB.QueryRowContext(ctx, "SELECT grantor_id,allowed FROM raid_substitutes WHERE player_id=? AND raid_id=?", id, raid).Scan(&from, &actions)
	if errors.Is(err, sql.ErrNoRows) {
		return base, nil
	}
	if err != nil {
		return Effective{}, err
	}
	authority, err := grantor(ctx, r.DB, from, raid)
	if err != nil {
		return Effective{}, err
	}
	return Effective{Allowed: p.Allowed & actions & authority, Source: "substitute"}, nil
}
func grantor(ctx context.Context, db bun.IDB, id, raid int64) (Actions, error) {
	role, err := staff(ctx, db, id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if role == validation.RoleAdmin {
		return All, nil
	}
	if role != validation.RoleRaidleader {
		return 0, nil
	}
	own, err := leader(ctx, db, id, raid)
	if err != nil || !own {
		return 0, err
	}
	p, err := policy(ctx, db, id)
	return Base(role, p, true).Allowed, err
}
func eligible(role validation.Role) bool {
	return role == validation.RoleRaidassistent || role == validation.RoleRaidleader
}
func record(ctx context.Context, tx bun.Tx, actor int64, key, action string, before, after any) error {
	b, err := json.Marshal(before)
	if err != nil {
		return err
	}
	a, err := json.Marshal(after)
	if err != nil {
		return err
	}
	_, err = tx.NewInsert().Model(&audit.Log{ActorID: &actor, EntityID: key, EntityType: "raid_permission", Action: action, BeforeJSON: string(b), AfterJSON: string(a), CreatedAt: time.Now().UTC()}).Exec(ctx)
	return err
}
func (r Repo) SetPolicy(ctx context.Context, actor, target int64, p Policy) error {
	return r.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		role, err := staff(ctx, tx, actor)
		if err != nil {
			return err
		}
		if role != validation.RoleAdmin {
			return ErrForbidden
		}
		role, err = staff(ctx, tx, target)
		if err != nil {
			return err
		}
		if !eligible(role) || p.Allowed & ^All != 0 {
			return ErrInvalid
		}
		old, err := policy(ctx, tx, target)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO raid_permission_policies(player_id,allowed,assigned_only) VALUES (?,?,?) ON CONFLICT(player_id) DO UPDATE SET allowed=excluded.allowed,assigned_only=excluded.assigned_only", target, p.Allowed, p.AssignedOnly)
		if err != nil {
			return err
		}
		return record(ctx, tx, actor, fmt.Sprint(target), "set_policy", old, p)
	})
}

type Grant struct {
	PlayerID  int64   `json:"player_id"`
	GrantorID int64   `json:"grantor_id"`
	Allowed   Actions `json:"allowed"`
}

func (r Repo) Delegable(ctx context.Context, id, raid int64) (Actions, error) {
	return grantor(ctx, r.DB, id, raid)
}
func (r Repo) Grants(ctx context.Context, raid int64) ([]Grant, error) {
	list := []Grant{}
	err := r.DB.NewSelect().Table("raid_substitutes").Column("player_id", "grantor_id", "allowed").Where("raid_id=?", raid).Order("player_id").Scan(ctx, &list)
	return list, err
}
func (r Repo) SetGrant(ctx context.Context, actor, target, raid int64, actions Actions) error {
	return r.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if actor == target {
			return ErrInvalid
		}
		var exists int
		if err := tx.QueryRowContext(ctx, "SELECT 1 FROM raids WHERE id=?", raid).Scan(&exists); err != nil {
			return err
		}
		role, err := staff(ctx, tx, actor)
		if err != nil {
			return err
		}
		authority, err := grantor(ctx, tx, actor, raid)
		if err != nil {
			return err
		}
		if authority == 0 || actions & ^authority != 0 {
			return ErrForbidden
		}
		var old Grant
		err = tx.QueryRowContext(ctx, "SELECT player_id,grantor_id,allowed FROM raid_substitutes WHERE raid_id=? AND player_id=?", raid, target).Scan(&old.PlayerID, &old.GrantorID, &old.Allowed)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if old.GrantorID != 0 && old.GrantorID != actor && role != validation.RoleAdmin {
			return ErrForbidden
		}
		if actions == 0 {
			_, err = tx.ExecContext(ctx, "DELETE FROM raid_substitutes WHERE raid_id=? AND player_id=?", raid, target)
		} else {
			recipient, e := staff(ctx, tx, target)
			if e != nil {
				return e
			}
			if !eligible(recipient) {
				return ErrInvalid
			}
			_, err = tx.ExecContext(ctx, "INSERT INTO raid_substitutes(raid_id,player_id,grantor_id,allowed) VALUES (?,?,?,?) ON CONFLICT(raid_id,player_id) DO UPDATE SET grantor_id=excluded.grantor_id,allowed=excluded.allowed", raid, target, actor, actions)
		}
		if err != nil {
			return err
		}
		return record(ctx, tx, actor, fmt.Sprintf("%d/%d", raid, target), "set_substitute", old, Grant{target, actor, actions})
	})
}
