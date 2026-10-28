package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/uptrace/bun"
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

func (r *Repo) Insert(ctx context.Context, l *Log) error {
	_, err := r.db.NewInsert().Model(l).Exec(ctx)
	return err
}

func (r *Repo) ListByEntity(ctx context.Context, entityType, entityID string) ([]Log, error) {
	var logs []Log
	err := r.db.NewSelect().Model(&logs).
		Where("entity_type = ?", entityType).
		Where("entity_id = ?", entityID).
		Order("id ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *Repo) List(ctx context.Context) ([]Entry, error) {
	var entries []Entry
	err := r.db.NewSelect().
		TableExpr("audit_log AS audit").
		ColumnExpr("audit.id, audit.actor_id, players.name AS actor_name, audit.entity_type, audit.entity_id, audit.action, audit.before_json, audit.after_json, audit.reason, audit.created_at").
		Join("LEFT JOIN players ON players.id = audit.actor_id").
		OrderExpr("audit.created_at DESC, audit.id DESC").
		Scan(ctx, &entries)
	if err != nil {
		return nil, err
	}
	hideAnonymised(entries)
	return entries, nil
}

// ActionPlayerAnonymise is the action of the entry that marks a player as anonymised.
const ActionPlayerAnonymise = "player_anonymise"

// hideAnonymised removes the name of anonymised players from the display (FR-30.4):
// the actor name, and name and login in the snapshots of the player record itself.
// The stored rows stay untouched.
func hideAnonymised(entries []Entry) {
	anonymised := map[string]bool{}
	for _, e := range entries {
		if e.EntityType == "player" && e.Action == ActionPlayerAnonymise {
			anonymised[e.EntityID] = true
		}
	}
	if len(anonymised) == 0 {
		return
	}
	for i := range entries {
		e := &entries[i]
		if e.ActorID != nil && anonymised[strconv.FormatInt(*e.ActorID, 10)] {
			e.ActorName = nil
		}
		if e.EntityType == "player" && anonymised[e.EntityID] {
			e.BeforeJSON = stripPlayerIdentity(e.BeforeJSON)
			e.AfterJSON = stripPlayerIdentity(e.AfterJSON)
		}
	}
}

func stripPlayerIdentity(snapshot string) string {
	var fields map[string]any
	if json.Unmarshal([]byte(snapshot), &fields) != nil || fields == nil {
		return snapshot
	}
	delete(fields, "name")
	delete(fields, "username")
	out, err := json.Marshal(fields)
	if err != nil {
		return snapshot
	}
	return string(out)
}

// Record writes one append-only entry with JSON snapshots of the record before
// and after the change. A nil snapshot is stored as JSON null. actorID 0 means
// the system acted without a signed-in player.
func (r *Repo) Record(ctx context.Context, actorID int64, entityType string, entityID int64, action string, before, after any, reason string) error {
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		return fmt.Errorf("marshal audit before snapshot: %w", err)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return fmt.Errorf("marshal audit after snapshot: %w", err)
	}
	entry := &Log{EntityType: entityType, EntityID: strconv.FormatInt(entityID, 10), Action: action, BeforeJSON: string(beforeJSON), AfterJSON: string(afterJSON), Reason: strings.TrimSpace(reason), CreatedAt: time.Now().UTC()}
	if actorID > 0 {
		entry.ActorID = &actorID
	}
	return r.Insert(ctx, entry)
}
