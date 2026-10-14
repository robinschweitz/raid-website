package legacyimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

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
	fromSheet, err := r.provenanceScope(ctx, source, sheetID)
	if err != nil {
		return nil, err
	}
	row := new(raidMapping)
	err = r.db.NewSelect().Model(row).Where("source = ?", source).Where("(sheet_id = ? OR sheet_id = ?)", sheetID, fromSheet).Where("raid_key = ?", key).Scan(ctx)
	return row, err
}

func (r *Repo) insertRaidMapping(ctx context.Context, row *raidMapping) error {
	_, err := r.db.NewInsert().Model(row).Exec(ctx)
	return err
}

func (r *Repo) mappings(ctx context.Context, source, sheetID string) ([]lootMapping, error) {
	fromSheet, err := r.provenanceScope(ctx, source, sheetID)
	if err != nil {
		return nil, err
	}
	var rows []lootMapping
	err = r.db.NewSelect().Model(&rows).Where("source = ?", source).Where("(sheet_id = ? OR sheet_id = ?)", sheetID, fromSheet).Where("tab = ?", "Loot Archive").Scan(ctx)
	return rows, err
}

func (r *Repo) insertMapping(ctx context.Context, row *lootMapping) error {
	_, err := r.db.NewInsert().Model(row).Exec(ctx)
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
	fromSheet, err := r.provenanceScope(ctx, source, sheetID)
	if err != nil {
		return nil, err
	}
	row := new(setupMapping)
	err = r.db.NewSelect().Model(row).Where("source = ?", source).Where("(sheet_id = ? OR sheet_id = ?)", sheetID, fromSheet).Scan(ctx)
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

func (r *Repo) playerHasPointHistory(ctx context.Context, playerID int64) (bool, error) {
	return r.db.NewSelect().TableExpr("enhance_point_transactions").Where("player_id=?", playerID).Exists(ctx)
}

func (r *Repo) hasLegacyProvenanceGap(ctx context.Context, source, sheetID string) (bool, error) {
	from, err := r.provenanceScope(ctx, source, sheetID)
	if err != nil {
		return false, err
	}
	return r.db.NewSelect().Model((*Import)(nil)).Where("source=? AND (source_url=? OR source_url=?) AND status='MERGED' AND dry_run_report=''", source, sheetID, from).Exists(ctx)
}

func (r *Repo) charactersByName(ctx context.Context, name string) ([]character.Character, error) {
	var rows []character.Character
	err := r.db.NewSelect().Model(&rows).
		Where("character.active = ? AND (character.name = ? OR EXISTS (SELECT 1 FROM character_name_history AS history WHERE history.character_id = character.id AND (history.old_name = ? OR history.new_name = ?)) OR EXISTS (SELECT 1 FROM character_merge_aliases AS alias WHERE alias.target_character_id = character.id AND alias.source_name = ?))", true, name, name, name, name).
		Scan(ctx)
	return rows, err
}

func (r *Repo) inactiveCharacterNameExists(ctx context.Context, name string) (bool, error) {
	return r.db.NewSelect().Model((*character.Character)(nil)).Where("character.active = ? AND (character.name = ? OR EXISTS (SELECT 1 FROM character_name_history AS history WHERE history.character_id = character.id AND (history.old_name = ? OR history.new_name = ?)) OR EXISTS (SELECT 1 FROM character_merge_aliases AS alias WHERE alias.target_character_id = character.id AND alias.source_name = ?))", false, name, name, name, name).Exists(ctx)
}

func (r *Repo) isCharacterMergeAlias(ctx context.Context, name string, targetID int64) (bool, error) {
	return r.db.NewSelect().TableExpr("character_merge_aliases").Where("source_name = ? AND target_character_id = ?", name, targetID).Exists(ctx)
}

// transaction holds the application's single SQLite connection through planning and merge.
func (s *Service) transaction(ctx context.Context, commit bool, fn func(*Service) error) error {
	if _, nested := s.imports.db.(bun.Tx); nested {
		return fn(s)
	}
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

func (r *Repo) sourceBaseline(ctx context.Context, source, sheetID string) (SheetData, error) {
	var raw string
	err := r.db.NewSelect().TableExpr("sheet_source_baselines").ColumnExpr("data_json").Where("source = ? AND sheet_id = ?", source, sheetID).Scan(ctx, &raw)
	if err != nil {
		return SheetData{}, err
	}
	var data SheetData
	err = json.Unmarshal([]byte(raw), &data)
	return data, err
}

func (r *Repo) saveSourceBaseline(ctx context.Context, source, sheetID string, data SheetData) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, "INSERT INTO sheet_source_baselines(source,sheet_id,data_json) VALUES(?,?,?) ON CONFLICT(source,sheet_id) DO UPDATE SET data_json=excluded.data_json", source, sheetID, string(raw))
	return err
}

func (r *Repo) setupBaseline(ctx context.Context, source, sheetID string, raidID int64) (string, error) {
	var raw string
	err := r.db.NewSelect().TableExpr("sheet_setup_baselines").ColumnExpr("setup_csv").Where("source=? AND sheet_id=? AND raid_id=?", source, sheetID, raidID).Scan(ctx, &raw)
	return raw, err
}
// latestArchivedSetupBaseline returns the last explicitly synchronized setup
// for an earlier archived raid of this series. It is evidence that the live
// setup tab changed before the next raid can adopt it.
func (r *Repo) latestArchivedSetupBaseline(ctx context.Context, source, sheetID string, seriesID int64, before time.Time) (string, error) {
	var raw string
	err := r.db.NewSelect().
		TableExpr("sheet_setup_baselines AS baseline").
		Join("JOIN raids AS raid ON raid.id = baseline.raid_id").
		ColumnExpr("baseline.setup_csv").
		Where("baseline.source = ? AND baseline.sheet_id = ?", source, sheetID).
		Where("raid.series_id = ? AND raid.scheduled_start < ?", seriesID, before.UTC()).
		Where("raid.status = ? AND raid.archived_at IS NOT NULL", validation.RaidStatusCompleted).
		OrderExpr("raid.scheduled_start DESC, raid.id DESC").
		Limit(1).Scan(ctx, &raw)
	return raw, err
}

func (r *Repo) saveSetupBaseline(ctx context.Context, source, sheetID string, raidID int64, raw []byte) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO sheet_setup_baselines(source,sheet_id,raid_id,setup_csv) VALUES(?,?,?,?) ON CONFLICT(source,sheet_id,raid_id) DO UPDATE SET setup_csv=excluded.setup_csv", source, sheetID, raidID, string(raw))
	return err
}

func (r *Repo) pointVersion(ctx context.Context, playerID int64) (int64, error) {
	var id int64
	err := r.db.NewSelect().TableExpr("enhance_point_transactions").ColumnExpr("COALESCE(MAX(id),0)").Where("player_id=?", playerID).Scan(ctx, &id)
	return id, err
}

func (r *Repo) pointBaseline(ctx context.Context, source, sheetID string, playerID int64) (int64, error) {
	from, err := r.provenanceScope(ctx, source, sheetID)
	if err != nil {
		return 0, err
	}
	var id int64
	err = r.db.NewSelect().TableExpr("sheet_point_baselines").ColumnExpr("transaction_id").Where("source=? AND (sheet_id=? OR sheet_id=?) AND player_id=?", source, sheetID, from, playerID).OrderExpr("CASE WHEN sheet_id=? THEN 0 ELSE 1 END", sheetID).Limit(1).Scan(ctx, &id)
	return id, err
}

func (r *Repo) savePointBaseline(ctx context.Context, source, sheetID string, playerID int64) error {
	id, err := r.pointVersion(ctx, playerID)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, "INSERT INTO sheet_point_baselines(source,sheet_id,player_id,transaction_id) VALUES(?,?,?,?) ON CONFLICT(source,sheet_id,player_id) DO UPDATE SET transaction_id=excluded.transaction_id", source, sheetID, playerID, id)
	return err
}

func (r *Repo) hasWebsitePoints(ctx context.Context, playerID int64) (bool, error) {
	return r.db.NewSelect().TableExpr("enhance_point_transactions").Where("player_id=? AND type <> 'legacy_open'", playerID).Exists(ctx)
}

func (r *Repo) unmappedLootExists(ctx context.Context, source, sheetID string, raidID int64) (bool, error) {
	from, err := r.provenanceScope(ctx, source, sheetID)
	if err != nil {
		return false, err
	}
	return r.db.NewSelect().TableExpr("loot_awards AS award").Where("award.raid_id=? AND NOT EXISTS (SELECT 1 FROM legacy_loot_mappings AS mapping WHERE mapping.source=? AND (mapping.sheet_id=? OR mapping.sheet_id=?) AND mapping.loot_award_id=award.id)", raidID, source, sheetID, from).Exists(ctx)
}

// copyImportProvenance retains the original immutable mappings. It only adds a
// scope for the authorized copy; no remote source is accessed here.
func (r *Repo) copyImportProvenance(ctx context.Context, source, fromSheet, toSheet string) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO sheet_provenance_scopes(source,sheet_id,from_sheet_id) VALUES(?,?,?)", source, toSheet, fromSheet)
	return err
}

func (r *Repo) provenanceScope(ctx context.Context, source, sheetID string) (string, error) {
	var from string
	err := r.db.NewSelect().TableExpr("sheet_provenance_scopes").ColumnExpr("from_sheet_id").Where("source=? AND sheet_id=?", source, sheetID).Scan(ctx, &from)
	if errors.Is(err, sql.ErrNoRows) {
		return sheetID, nil
	}
	return from, err
}

func (r *Repo) raidMappings(ctx context.Context, source, sheetID string) ([]raidMapping, error) {
	from, err := r.provenanceScope(ctx, source, sheetID)
	if err != nil {
		return nil, err
	}
	var rows []raidMapping
	err = r.db.NewSelect().Model(&rows).Where("source=? AND (sheet_id=? OR sheet_id=?)", source, sheetID, from).Scan(ctx, &rows)
	return rows, err
}
