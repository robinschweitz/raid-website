package push

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"raid-website/internal/validation"
)

// Repo reads and writes push_subscriptions.
type Repo struct {
	db bun.IDB
}

// NewRepo wires the repository.
func NewRepo(db bun.IDB) *Repo { return &Repo{db: db} }

// WithTx returns a repository bound to tx.
func (r *Repo) WithTx(tx bun.Tx) *Repo { return &Repo{db: tx} }

// Transaction runs fn in one transaction.
func (r *Repo) Transaction(ctx context.Context, fn func(*Repo, bun.Tx) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return fn(r.WithTx(tx), tx)
	})
}

// GetByEndpoint returns the subscription of a browser endpoint.
func (r *Repo) GetByEndpoint(ctx context.Context, endpoint string) (*Subscription, error) {
	sub := new(Subscription)
	err := r.db.NewSelect().Model(sub).Where("endpoint = ?", endpoint).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return sub, nil
}

// Insert stores a new subscription.
func (r *Repo) Insert(ctx context.Context, sub *Subscription) error {
	_, err := r.db.NewInsert().Model(sub).Exec(ctx)
	return err
}

// Delete removes one subscription.
func (r *Repo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.NewDelete().Model((*Subscription)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

// ListByPlayer returns a player's subscriptions, newest first.
func (r *Repo) ListByPlayer(ctx context.Context, playerID int64) ([]Subscription, error) {
	subs := []Subscription{}
	err := r.db.NewSelect().Model(&subs).Where("player_id = ?", playerID).Order("created_at DESC", "id DESC").Scan(ctx)
	return subs, err
}

// TrimPlayer deletes a player's oldest subscriptions beyond keep and returns
// what it deleted.
func (r *Repo) TrimPlayer(ctx context.Context, playerID int64, keep int) ([]Subscription, error) {
	subs, err := r.ListByPlayer(ctx, playerID)
	if err != nil || len(subs) <= keep {
		return nil, err
	}
	dropped := subs[keep:]
	for _, sub := range dropped {
		if err := r.Delete(ctx, sub.ID); err != nil {
			return nil, err
		}
	}
	return dropped, nil
}

// ListRecipients returns the subscriptions of active players with a login and
// at least the MEMBER role, except the actor. With playerIDs only those
// players are considered.
func (r *Repo) ListRecipients(ctx context.Context, actorID int64, playerIDs []int64) ([]Subscription, error) {
	subs := []Subscription{}
	q := r.db.NewSelect().Model(&subs).
		Join("JOIN players AS p ON p.id = subscription.player_id").
		Where("p.active = ?", true).
		Where("p.username IS NOT NULL").
		Where("p.role IN (?)", bun.List([]validation.Role{validation.RoleMember, validation.RoleRaidassistent, validation.RoleRaidleader, validation.RoleAdmin})).
		Where("subscription.player_id <> ?", actorID).
		Order("subscription.id ASC")
	if playerIDs != nil {
		q = q.Where("subscription.player_id IN (?)", bun.List(playerIDs))
	}
	err := q.Scan(ctx)
	return subs, err
}
