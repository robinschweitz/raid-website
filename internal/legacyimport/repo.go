package legacyimport

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
	"raid-website/internal/character"
	"raid-website/internal/raidcorrection"
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

func (r *Repo) Insert(ctx context.Context, imp *Import) error {
	_, err := r.db.NewInsert().Model(imp).Exec(ctx)
	return err
}

func (r *Repo) Get(ctx context.Context, id int64) (*Import, error) {
	imp := new(Import)
	err := r.db.NewSelect().Model(imp).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return imp, nil
}

func (r *Repo) FindByHash(ctx context.Context, source, sheetID, hash string) (*Import, error) {
	imp := new(Import)
	err := r.db.NewSelect().Model(imp).
		Where("source = ?", source).
		Where("source_url = ?", sheetID).
		Where("raw_hash = ?", hash).
		Where("status = ?", "MERGED").
		Order("id DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return imp, nil
}

type lootMapping struct {
	bun.BaseModel `bun:"table:legacy_loot_mappings"`
	ID            int64 `bun:"id,pk,autoincrement"`
	Source        string
	SheetID       string
	Tab           string
	Identity      string
	ExternalID    string
	Fingerprint   string
	LootAwardID   int64
}

type raidMapping struct {
	bun.BaseModel `bun:"table:legacy_raid_mappings"`
	Source        string `bun:",pk"`
	SheetID       string `bun:",pk"`
	RaidKey       string `bun:",pk"`
	RaidID        int64
}

func (r *Repo) raidMapping(ctx context.Context, source, sheetID, key string) (*raidMapping, error) {
	row := new(raidMapping)
	err := r.db.NewSelect().Model(row).Where("source = ?", source).Where("sheet_id = ?", sheetID).Where("raid_key = ?", key).Scan(ctx)
	return row, err
}

func (r *Repo) raidMappingByRaidID(ctx context.Context, source, sheetID string, raidID int64) (*raidMapping, error) {
	row := new(raidMapping)
	err := r.db.NewSelect().Model(row).Where("source = ?", source).Where("sheet_id = ?", sheetID).Where("raid_id = ?", raidID).Scan(ctx)
	return row, err
}

type syncRaidMapping struct {
	RaidID int64 `bun:"raid_id"`
}

func (r *Repo) syncRaidMapping(ctx context.Context, source, sheetID, key string) (*syncRaidMapping, error) {
	row := new(syncRaidMapping)
	err := r.db.NewSelect().TableExpr("legacy_sync_raid_mappings").Column("raid_id").Where("source = ? AND sheet_id = ? AND raid_key = ?", source, sheetID, key).Scan(ctx, row)
	return row, err
}

func (r *Repo) syncRaidMappingByRaidID(ctx context.Context, source, sheetID string, raidID int64) (string, error) {
	var key string
	err := r.db.NewSelect().TableExpr("legacy_sync_raid_mappings").Column("raid_key").Where("source = ? AND sheet_id = ? AND raid_id = ?", source, sheetID, raidID).Scan(ctx, &key)
	return key, err
}

func (r *Repo) insertSyncRaidMapping(ctx context.Context, source, sheetID, key string, raidID, actorID int64, now time.Time) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO legacy_sync_raid_mappings (source, sheet_id, raid_key, raid_id, adopted_by_player_id, adopted_at) VALUES (?, ?, ?, ?, ?, ?)", source, sheetID, key, raidID, actorID, now)
	return err
}

func (r *Repo) ensureSyncRaidMapping(ctx context.Context, source, sheetID, key string, raidID, actorID int64, now time.Time) error {
	mapping, err := r.syncRaidMapping(ctx, source, sheetID, key)
	if err == nil {
		if mapping.RaidID != raidID {
			return fmt.Errorf("sync raid key %q is already mapped to raid %d", key, mapping.RaidID)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return r.insertSyncRaidMapping(ctx, source, sheetID, key, raidID, actorID, now)
}

func (r *Repo) syncPlayerID(ctx context.Context, source, sheetID, name string) (int64, error) {
	var id int64
	err := r.db.NewSelect().TableExpr("legacy_sync_player_mappings").Column("player_id").Where("source = ? AND sheet_id = ? AND player_name = ?", source, sheetID, name).Scan(ctx, &id)
	return id, err
}

func (r *Repo) insertSyncPlayerMapping(ctx context.Context, source, sheetID, name string, playerID, actorID int64, now time.Time) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO legacy_sync_player_mappings (source, sheet_id, player_name, player_id, adopted_by_player_id, adopted_at) VALUES (?, ?, ?, ?, ?, ?)", source, sheetID, name, playerID, actorID, now)
	return err
}

func (r *Repo) syncCharacterID(ctx context.Context, source, sheetID, name, playerName string) (int64, error) {
	var id int64
	err := r.db.NewSelect().TableExpr("legacy_sync_character_mappings").Column("character_id").Where("source = ? AND sheet_id = ? AND character_name = ? AND player_name = ?", source, sheetID, name, playerName).Scan(ctx, &id)
	return id, err
}

func (r *Repo) insertSyncCharacterMapping(ctx context.Context, source, sheetID, name, playerName string, characterID, playerID, actorID int64, now time.Time) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO legacy_sync_character_mappings (source, sheet_id, character_name, player_name, character_id, player_id, adopted_by_player_id, adopted_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", source, sheetID, name, playerName, characterID, playerID, actorID, now)
	return err
}

func (r *Repo) insertRaidMapping(ctx context.Context, row *raidMapping) error {
	_, err := r.db.NewInsert().Model(row).Exec(ctx)
	return err
}

func (r *Repo) mappings(ctx context.Context, source, sheetID string) ([]lootMapping, error) {
	var rows []lootMapping
	err := r.db.NewSelect().Model(&rows).Where("source = ?", source).Where("sheet_id = ?", sheetID).Where("tab = ?", "Loot Archive").Scan(ctx)
	return rows, err
}

func (r *Repo) insertMapping(ctx context.Context, row *lootMapping) error {
	_, err := r.db.NewInsert().Model(row).Exec(ctx)
	return err
}

type lootSyncState struct {
	bun.BaseModel    `bun:"table:legacy_loot_sync_states"`
	ID               int64 `bun:"id,pk,autoincrement"`
	Source           string
	SheetID          string
	RaidKey          string
	RawHash          string
	SyncedByPlayerID int64
	SyncedAt         time.Time
	SyncRaidID       *int64
}

func (r *Repo) insertLootSyncState(ctx context.Context, row *lootSyncState) error {
	_, err := r.db.NewInsert().Model(row).Exec(ctx)
	return err
}

func (r *Repo) insertLootSyncEvent(ctx context.Context, stateID, mappingID, awardID int64, eventType string, actorID int64, reason string, now time.Time) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO legacy_loot_sync_events (sync_state_id, legacy_loot_mapping_id, loot_award_id, event_type, actor_id, reason, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)", stateID, mappingID, awardID, eventType, actorID, reason, now)
	return err
}

func (r *Repo) recordPreview(ctx context.Context, source, sheetID, hash string) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO import_previews (source, sheet_id, raw_hash) VALUES (?, ?, ?) ON CONFLICT DO NOTHING", source, sheetID, hash)
	return err
}

func (r *Repo) hasPreview(ctx context.Context, source, sheetID, hash string) (bool, error) {
	return r.db.NewSelect().TableExpr("import_previews").Where("source = ?", source).Where("sheet_id = ?", sheetID).Where("raw_hash = ?", hash).Exists(ctx)
}

func (r *Repo) writeAudit(ctx context.Context, row *audit.Log) error {
	return audit.NewRepo(r.db).Insert(ctx, row)
}

type setupMapping struct {
	bun.BaseModel `bun:"table:legacy_setup_mappings"`
	Source        string `bun:",pk"`
	SheetID       string `bun:",pk"`
	SeriesID      int64
	RaidID        int64
}

func (r *Repo) setupMapping(ctx context.Context, source, sheetID string) (*setupMapping, error) {
	row := new(setupMapping)
	err := r.db.NewSelect().Model(row).Where("source = ?", source).Where("sheet_id = ?", sheetID).Scan(ctx)
	return row, err
}

func (r *Repo) insertSetupMapping(ctx context.Context, row *setupMapping) error {
	_, err := r.db.NewInsert().Model(row).Exec(ctx)
	return err
}

func (r *Repo) pointTransactionCount(ctx context.Context) (int, error) {
	count, err := r.db.NewSelect().TableExpr("enhance_point_transactions").Count(ctx)
	return int(count), err
}

func (r *Repo) hasLegacyProvenanceGap(ctx context.Context, source, sheetID string) (bool, error) {
	return r.db.NewSelect().Model((*Import)(nil)).Where("source = ?", source).Where("source_url = ?", sheetID).Where("status = ?", "MERGED").Where("dry_run_report = ?", "").Exists(ctx)
}

func (r *Repo) charactersByName(ctx context.Context, name string) ([]character.Character, error) {
	var rows []character.Character
	err := r.db.NewSelect().Model(&rows).
		Where("character.active = ? AND (character.name = ? OR EXISTS (SELECT 1 FROM character_name_history AS history WHERE history.character_id = character.id AND (history.old_name = ? OR history.new_name = ?)) OR EXISTS (SELECT 1 FROM character_merge_aliases AS alias WHERE alias.target_character_id = character.id AND alias.source_name = ?))", true, name, name, name, name).
		Scan(ctx)
	return rows, err
}

func (r *Repo) charactersExactByName(ctx context.Context, name string) ([]character.Character, error) {
	var rows []character.Character
	err := r.db.NewSelect().Model(&rows).Where("character.active = ? AND character.name = ?", true, name).Scan(ctx)
	return rows, err
}

func (r *Repo) isCharacterMergeAlias(ctx context.Context, name string, targetID int64) (bool, error) {
	return r.db.NewSelect().TableExpr("character_merge_aliases").Where("source_name = ? AND target_character_id = ?", name, targetID).Exists(ctx)
}

// transaction holds the application's single SQLite connection through planning and merge.
func (s *Service) transaction(ctx context.Context, commit bool, fn func(*Service) error) error {
	tx, err := s.imports.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	bound := *s
	bound.imports = s.imports.WithTx(tx)
	bound.players = s.players.WithTx(tx)
	bound.chars = s.chars.WithTx(tx)
	bound.raids = s.raids.WithTx(tx)
	bound.att = s.att.WithTx(tx)
	bound.loot = s.loot.WithTx(tx)
	bound.points = s.points.WithTx(tx)
	bound.series = s.series.WithTx(tx)
	bound.corrections = raidcorrection.NewService(raidcorrection.NewRepo(tx))
	if err := fn(&bound); err != nil {
		return err
	}
	if commit {
		return tx.Commit()
	}
	return nil
}

func (r *Repo) List(ctx context.Context) ([]Import, error) {
	var imports []Import
	err := r.db.NewSelect().Model(&imports).Order("id DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return imports, nil
}
