package accountmerge

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/player"
	"raid-website/internal/validation"
)

type Repo struct {
	db bun.IDB
}

func NewRepo(db bun.IDB) *Repo { return &Repo{db: db} }

func (r *Repo) InTransaction(ctx context.Context, fn func(*Repo) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Reserve the SQLite writer before checking cross-account lock conflicts.
		if _, err := tx.ExecContext(ctx, "UPDATE players SET id = id WHERE id = -1"); err != nil {
			return err
		}
		return fn(NewRepo(tx))
	})
}

func (r *Repo) GetPlayer(ctx context.Context, id int64) (*player.Player, error) {
	p := new(player.Player)
	err := r.db.NewSelect().Model(p).Where("id = ?", id).Scan(ctx)
	return p, err
}

func (r *Repo) CountActiveAdmins(ctx context.Context) (int, error) {
	return player.NewRepo(r.db).CountActiveAdmins(ctx)
}

func (r *Repo) ListCharacters(ctx context.Context, playerID int64) ([]character.Character, error) {
	var rows []character.Character
	err := r.db.NewSelect().Model(&rows).Where("player_id = ?", playerID).Order("id ASC").Scan(ctx)
	return rows, err
}

func (r *Repo) UpdateCharacterOwner(ctx context.Context, id, playerID int64, updatedAt time.Time) error {
	_, err := r.db.NewUpdate().Table("characters").Set("player_id = ?", playerID).Set("updated_at = ?", updatedAt).Where("id = ?", id).Exec(ctx)
	return err
}

func (r *Repo) ListLocks(ctx context.Context, playerID int64) ([]enhancepoints.ItemLock, error) {
	var rows []enhancepoints.ItemLock
	err := r.db.NewSelect().Model(&rows).Where("player_id = ?", playerID).Order("id ASC").Scan(ctx)
	return rows, err
}

func (r *Repo) ListActiveLocks(ctx context.Context, playerID int64) ([]enhancepoints.ItemLock, error) {
	var rows []enhancepoints.ItemLock
	err := r.db.NewSelect().Model(&rows).Where("player_id = ?", playerID).Where("status = ?", validation.LockStatusActive).Order("id ASC").Scan(ctx)
	return rows, err
}

func (r *Repo) UpdateLockOwner(ctx context.Context, id, playerID int64, updatedAt time.Time) error {
	_, err := r.db.NewUpdate().Table("item_locks").Set("player_id = ?", playerID).Set("updated_at = ?", updatedAt).Where("id = ?", id).Exec(ctx)
	return err
}

func (r *Repo) Balance(ctx context.Context, playerID int64) (int, error) {
	var balance int
	err := r.db.NewSelect().TableExpr("enhance_point_transactions").ColumnExpr("COALESCE(SUM(amount), 0)").Where("player_id = ?", playerID).Scan(ctx, &balance)
	return balance, err
}

func (r *Repo) InsertPointTransaction(ctx context.Context, row *enhancepoints.Transaction) error {
	_, err := r.db.NewInsert().Model(row).Exec(ctx)
	return err
}

func (r *Repo) RevokeSourceAuth(ctx context.Context, playerID int64) (map[string]int64, error) {
	tables := []string{"sessions", "webauthn_sessions", "mfa_login_challenges", "mfa_recovery_codes", "webauthn_credentials", "player_mfa_settings", "push_subscriptions"}
	counts := make(map[string]int64, len(tables))
	for _, table := range tables {
		result, err := r.db.NewDelete().Table(table).Where("player_id = ?", playerID).Exec(ctx)
		if err != nil {
			return nil, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		counts[table] = count
	}
	return counts, nil
}

func (r *Repo) MoveHistoricalReferences(ctx context.Context, sourceID, targetID int64) error {
	// Keep both awards when an actor-scoped request ID collides after the merge.
	if _, err := r.db.NewUpdate().Table("loot_awards").Set("request_id = 'account-merge-' || ? || '-' || id", sourceID).Where("awarded_by_player_id = ?", sourceID).Where("request_id IS NOT NULL").Where("EXISTS (SELECT 1 FROM loot_awards AS target WHERE target.awarded_by_player_id = ? AND target.request_id = loot_awards.request_id)", targetID).Exec(ctx); err != nil {
		return err
	}
	// Poll votes and quick-add requests have per-player unique keys. The target's existing
	// key wins; the source key cannot survive after its player is deleted.
	if _, err := r.db.NewDelete().Table("poll_votes").Where("player_id = ?", sourceID).Where("EXISTS (SELECT 1 FROM poll_votes AS target WHERE target.poll_id = poll_votes.poll_id AND target.player_id = ?)", targetID).Exec(ctx); err != nil {
		return err
	}
	if _, err := r.db.NewDelete().Table("raid_quick_add_requests").Where("actor_id = ?", sourceID).Where("EXISTS (SELECT 1 FROM raid_quick_add_requests AS target WHERE target.raid_id = raid_quick_add_requests.raid_id AND target.actor_id = ? AND target.request_id = raid_quick_add_requests.request_id)", targetID).Exec(ctx); err != nil {
		return err
	}
	statements := []struct{ table, column string }{
		{"character_name_history", "actor_id"}, {"raid_series", "raid_leader_id"}, {"raids", "raid_leader_id"},
		{"raid_attendance", "player_id"}, {"raid_signups", "player_id"}, {"character_wishlist", "player_id"},
		{"loot_awards", "awarded_to_player_id"}, {"loot_awards", "awarded_by_player_id"}, {"enhance_point_transactions", "player_id"},
		{"enhance_point_transactions", "actor_id"}, {"item_locks", "actor_id"}, {"polls", "created_by"},
		{"poll_votes", "player_id"}, {"audit_log", "actor_id"},
		{"raid_quick_add_requests", "actor_id"}, {"registration_requests", "requester_player_id"},
		{"registration_codes", "created_by_player_id"}, {"registration_codes", "used_by_player_id"},
		{"raid_boss_kills", "created_by_player_id"}, {"raid_boss_kills", "removed_by_player_id"},
	}
	for _, statement := range statements {
		if _, err := r.db.NewUpdate().Table(statement.table).Set(statement.column+" = ?", targetID).Where(statement.column+" = ?", sourceID).Exec(ctx); err != nil {
			return fmt.Errorf("move %s.%s: %w", statement.table, statement.column, err)
		}
	}
	return nil
}

func (r *Repo) DeletePlayer(ctx context.Context, id int64) error {
	_, err := r.db.NewDelete().Table("players").Where("id = ?", id).Exec(ctx)
	return err
}

func (r *Repo) Audit(ctx context.Context, actorID int64, entityType string, entityID any, action string, before, after any, reason string, now time.Time) error {
	beforeJSON, err := auditJSON(before)
	if err != nil {
		return fmt.Errorf("marshal audit before: %w", err)
	}
	afterJSON, err := auditJSON(after)
	if err != nil {
		return fmt.Errorf("marshal audit after: %w", err)
	}
	entity := fmt.Sprint(entityID)
	if id, ok := entityID.(int64); ok {
		entity = strconv.FormatInt(id, 10)
	}
	_, err = r.db.NewInsert().Model(&audit.Log{ActorID: &actorID, EntityType: entityType, EntityID: entity, Action: action, BeforeJSON: beforeJSON, AfterJSON: afterJSON, Reason: reason, CreatedAt: now}).Exec(ctx)
	return err
}

// AnonymisedPlayer reports whether an anonymise audit entry exists for the player.
func (r *Repo) AnonymisedPlayer(ctx context.Context, id int64) (bool, error) {
	return r.db.NewSelect().Table("audit_log").Where("entity_type = 'player'").Where("entity_id = ?", strconv.FormatInt(id, 10)).Where("action = ?", AnonymiseAction).Exists(ctx)
}

// NameSource returns the character whose name replaces the player name: the oldest
// main, else the oldest character. It returns nil when the player has no character.
func (r *Repo) NameSource(ctx context.Context, playerID int64) (*character.Character, error) {
	var rows []character.Character
	err := r.db.NewSelect().Model(&rows).Where("player_id = ?", playerID).
		OrderExpr("is_main DESC, created_at ASC, id ASC").Limit(1).Scan(ctx)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

// AnonymiseAccount removes the login, credentials and Rising-Gods profile data of a
// player and sets the replacement name. It returns the number of rows removed per table.
func (r *Repo) AnonymiseAccount(ctx context.Context, p *player.Player, newName string, now time.Time) (map[string]int64, error) {
	counts, err := r.RevokeSourceAuth(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	if p.Username != nil {
		result, err := r.db.NewDelete().Table("login_attempts").Where("username = ?", strings.ToLower(strings.TrimSpace(*p.Username))).Exec(ctx)
		if err != nil {
			return nil, err
		}
		if counts["login_attempts"], err = result.RowsAffected(); err != nil {
			return nil, err
		}
	}
	// Unused registration codes of the player's requests must not outlive the erasure.
	if _, err := r.db.NewUpdate().Table("registration_codes").Set("revoked_at = ?", now).
		Where("revoked_at IS NULL").Where("used_at IS NULL").
		Where("request_id IN (SELECT id FROM registration_requests WHERE requester_player_id = ?)", p.ID).Exec(ctx); err != nil {
		return nil, err
	}
	if _, err := r.db.NewUpdate().Table("registration_requests").
		Set("raw_profile_json = ''").Set("external_profile_url = ''").Set("external_profile_id = 'anonymised-' || id").
		Set("character_name = ''").Set("race = ''").Set("updated_at = ?", now).
		Where("requester_player_id = ?", p.ID).Exec(ctx); err != nil {
		return nil, err
	}
	if _, err := r.db.NewUpdate().Table("registration_requests").
		Set("raw_profile_json = ''").Set("external_profile_url = ''").Set("external_profile_id = 'anonymised-' || id").
		Set("character_name = ''").Set("race = ''").Set("updated_at = ?", now).
		Where("id IN (SELECT request_id FROM registration_codes WHERE used_by_player_id = ?)", p.ID).Exec(ctx); err != nil {
		return nil, err
	}
	result, err := r.db.NewUpdate().Table("characters").
		Set("external_source = ''").Set("external_profile_id = ''").Set("external_profile_url = ''").Set("external_profile_json = ''").
		Where("player_id = ?", p.ID).Where("(external_source <> '' OR external_profile_id <> '' OR external_profile_url <> '' OR external_profile_json <> '')").Exec(ctx)
	if err != nil {
		return nil, err
	}
	if counts["character_profiles"], err = result.RowsAffected(); err != nil {
		return nil, err
	}
	_, err = r.db.NewUpdate().Table("players").Set("name = ?", newName).Set("username = NULL").Set("password_hash = ''").
		Set("role = ?", validation.RoleGuest).Set("updated_at = ?", now).Where("id = ?", p.ID).Exec(ctx)
	return counts, err
}

// ExportSections selects only explicitly allowed columns, never auth tables.
func (r *Repo) ExportSections(ctx context.Context, playerID int64) (map[string][]map[string]any, error) {
	sections := make(map[string][]map[string]any)
	for _, section := range []struct{ name, table, columns, where string }{
		{"characters", "characters", "id, player_id, name, class, main_spec, second_spec, gear_score, is_main, active, external_source, external_profile_id, external_profile_url, created_at, updated_at", "player_id = ?"},
		{"signups", "raid_signups", "id, raid_id, player_id, character_id, spec, is_withdrawn, created_at, updated_at", "player_id = ?"},
		{"absences", "raid_absences", "id, raid_id, player_id, reason, status, created_at", "player_id = ?"},
		{"roster", "raid_attendance", "id, raid_id, player_id, character_id, character_name, class, spec, role, is_substitute, is_bench, late_join, early_leave, notes, created_at, updated_at", "player_id = ?"},
		{"loot", "loot_awards", "id, raid_id, catalog_boss_id, boss, item_id, item_name, awarded_to_player_id, awarded_to_character_id, awarded_to_character_name, awarded_by_player_id, source, is_main_spec, award_method, enhanced_cost, item_lock_id, notes, awarded_at, withdrawn_at, withdrawn_by_player_id, withdrawn_reason, created_at, updated_at", "awarded_to_player_id = ?"},
		{"ep_transactions", "enhance_point_transactions", "id, player_id, raid_id, loot_award_id, amount, type, reason, ref_type, ref_id, actor_id, created_at", "player_id = ?"},
		{"item_locks", "item_locks", "id, player_id, raid_id, item_id, item_name, boss, status, locked_at, used_at, expired_at, cancelled_at, actor_id, notes, created_at, updated_at", "player_id = ?"},
		{"wishlist", "character_wishlist", "id, player_id, character_id, item_id, item_name, priority, notes, spec, is_enhanced, created_at, updated_at", "player_id = ?"},
		{"votes", "poll_votes", "id, poll_id, player_id, choice, voted_at", "player_id = ?"},
	} {
		rows := []map[string]any{}
		if err := r.db.NewSelect().TableExpr(section.table).ColumnExpr(section.columns).Where(section.where, playerID).OrderExpr("id ASC").Scan(ctx, &rows); err != nil {
			return nil, fmt.Errorf("export %s: %w", section.name, err)
		}
		sections[section.name] = rows
	}
	choices := []map[string]any{}
	if err := r.db.NewSelect().TableExpr("poll_vote_choices").ColumnExpr("id, vote_id, option_id, label, sort_order").Where("vote_id IN (SELECT id FROM poll_votes WHERE player_id = ?)", playerID).OrderExpr("id ASC").Scan(ctx, &choices); err != nil {
		return nil, err
	}
	sections["vote_choices"] = choices
	logs := []map[string]any{}
	if err := r.db.NewSelect().TableExpr("audit_log").ColumnExpr("id, actor_id, entity_type, entity_id, action, before_json, after_json, reason, created_at").Where(`actor_id = ? OR (entity_type = 'player' AND entity_id = ?) OR (entity_type = 'character' AND entity_id IN (SELECT CAST(id AS TEXT) FROM characters WHERE player_id = ?)) OR EXISTS (SELECT 1 FROM json_tree(CASE WHEN json_valid(before_json) THEN before_json ELSE '{}' END) WHERE key IN ('player_id','awarded_to_player_id','source_player_id','target_player_id') AND atom = ?) OR EXISTS (SELECT 1 FROM json_tree(CASE WHEN json_valid(after_json) THEN after_json ELSE '{}' END) WHERE key IN ('player_id','awarded_to_player_id','source_player_id','target_player_id') AND atom = ?)`, playerID, strconv.FormatInt(playerID, 10), playerID, playerID, playerID).OrderExpr("id ASC").Scan(ctx, &logs); err != nil {
		return nil, err
	}
	sections["audit"] = logs
	return sections, nil
}

// AuditExport records who requested the disclosure, without copying the file.
func (r *Repo) AuditExport(ctx context.Context, actorID, playerID int64) error {
	return audit.NewRepo(r.db).Record(ctx, actorID, "player", playerID, "export", nil, map[string]any{"player_id": playerID}, "")
}
