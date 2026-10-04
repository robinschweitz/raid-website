package legacyimport

import (
	"context"

	"github.com/uptrace/bun"

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

func (r *Repo) recordPreview(ctx context.Context, source, sheetID, hash string) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO import_previews (source, sheet_id, raw_hash) VALUES (?, ?, ?) ON CONFLICT DO NOTHING", source, sheetID, hash)
	return err
}

func (r *Repo) hasPreview(ctx context.Context, source, sheetID, hash string) (bool, error) {
	return r.db.NewSelect().TableExpr("import_previews").Where("source = ?", source).Where("sheet_id = ?", sheetID).Where("raw_hash = ?", hash).Exists(ctx)
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
		Where("character.name = ? OR EXISTS (SELECT 1 FROM character_name_history AS history WHERE history.character_id = character.id AND (history.old_name = ? OR history.new_name = ?))", name, name, name).
		Scan(ctx)
	return rows, err
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
	bound.tokens = s.tokens.WithTx(tx)
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
