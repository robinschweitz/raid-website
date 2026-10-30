package loot

import (
	"context"
	"strings"

	"github.com/uptrace/bun"

	"raid-website/internal/attendance"
	"raid-website/internal/audit"
	"raid-website/internal/bosskill"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/lootcatalog"
	"raid-website/internal/player"
	"raid-website/internal/raid"
)

type Repo struct {
	db bun.IDB
}

type awardRepositories struct {
	raids      *raid.Repo
	players    *player.Repo
	characters *character.Repo
	attendance *attendance.Repo
	points     *enhancepoints.Repo
	audit      *audit.Repo
}

func (r *Repo) awardRepositories() awardRepositories {
	return awardRepositories{raids: raid.NewRepo(r.db), players: player.NewRepo(r.db), characters: character.NewRepo(r.db), attendance: attendance.NewRepo(r.db), points: enhancepoints.NewRepo(r.db), audit: audit.NewRepo(r.db)}
}

func NewRepo(db bun.IDB) *Repo {
	return &Repo{db: db}
}

// Catalog binds validation to the award transaction.
func (r *Repo) Catalog() *lootcatalog.Service {
	return lootcatalog.NewService(lootcatalog.NewRepo(r.db))
}

func (r *Repo) WithTx(tx bun.Tx) *Repo {
	return &Repo{db: tx}
}

// InTransaction keeps loot and lock consumption atomic.
func (r *Repo) InTransaction(ctx context.Context, fn func(*Repo) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		repo := r.WithTx(tx)
		if err := repo.ReserveWriter(ctx); err != nil {
			return err
		}
		return fn(repo)
	})
}

// ReserveWriter obtains SQLite's writer reservation before any balance reads.
func (r *Repo) ReserveWriter(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, "UPDATE raids SET id = id WHERE id = -1")
	return err
}

// GetByRequest looks up the immutable original request association.
func (r *Repo) GetByRequest(ctx context.Context, actor int64, requestID string) (*Award, error) {
	a := new(Award)
	err := r.db.NewSelect().Model(a).Where("awarded_by_player_id = ? AND request_id = ?", actor, requestID).Scan(ctx)
	return a, err
}

// GetByRaidRequest finds the award a raid-scoped request ID recorded in this
// raid, whichever staff member wrote it.
func (r *Repo) GetByRaidRequest(ctx context.Context, raidID int64, requestID string) (*Award, error) {
	a := new(Award)
	err := r.db.NewSelect().Model(a).Where("raid_id = ? AND request_id = ?", raidID, requestID).Scan(ctx)
	return a, err
}

func (r *Repo) CreateAward(ctx context.Context, a *Award) error {
	if err := a.DeriveMainSpec(); err != nil {
		return err
	}
	_, err := r.db.NewInsert().Model(a).Exec(ctx)
	return err
}

func (r *Repo) Get(ctx context.Context, id int64) (*Award, error) {
	a := new(Award)
	err := r.db.NewSelect().Model(a).Where("id = ?", id).Scan(ctx)
	return a, err
}

func (r *Repo) Update(ctx context.Context, a *Award) error {
	if err := a.DeriveMainSpec(); err != nil {
		return err
	}
	_, err := r.db.NewUpdate().Model(a).ExcludeColumn("request_id", "request_hash").WherePK().Exec(ctx)
	return err
}

func (r *Repo) ListByRaid(ctx context.Context, raidID int64) ([]Award, error) {
	var as []Award
	err := r.db.NewSelect().Model(&as).
		Where("raid_id = ?", raidID).
		Order("id ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return as, nil
}

func (r *Repo) ListByPlayer(ctx context.Context, playerID int64) ([]Award, error) {
	var as []Award
	err := r.db.NewSelect().Model(&as).
		Where("awarded_to_player_id = ?", playerID).
		Order("awarded_at DESC", "id DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return as, nil
}

// FillCharacterNames sets the current character name on awards without a stored
// name. It changes only the given rows, never the database: website awards keep
// the character ID alone, and historical awards keep their stored name.
func (r *Repo) FillCharacterNames(ctx context.Context, awards []Award) error {
	var characterIDs []int64
	for _, award := range awards {
		if award.AwardedToCharacterID != nil && strings.TrimSpace(award.AwardedToCharacterName) == "" {
			characterIDs = append(characterIDs, *award.AwardedToCharacterID)
		}
	}
	if len(characterIDs) == 0 {
		return nil
	}
	var characters []character.Character
	err := r.db.NewSelect().Model(&characters).
		Column("id", "name").
		Where("id IN (?)", bun.List(characterIDs)).
		Scan(ctx)
	if err != nil {
		return err
	}
	names := make(map[int64]string, len(characters))
	for _, c := range characters {
		names[c.ID] = c.Name
	}
	for i := range awards {
		if awards[i].AwardedToCharacterID != nil && strings.TrimSpace(awards[i].AwardedToCharacterName) == "" {
			awards[i].AwardedToCharacterName = names[*awards[i].AwardedToCharacterID]
		}
	}
	return nil
}

func (r *Repo) PlayerHasMainSpecAward(ctx context.Context, raidID, playerID int64) (bool, error) {
	var awards []Award
	err := r.db.NewSelect().Model(&awards).
		Column("item_id", "item_name").
		Where("raid_id = ?", raidID).
		Where("awarded_to_player_id = ?", playerID).
		Where("is_main_spec = ?", true).
		Where("withdrawn_at IS NULL").
		Scan(ctx)
	if err != nil {
		return false, err
	}
	for _, award := range awards {
		// Tokens and Shadowfrost shards are tracked as loot but do not
		// disqualify a player from the +10 EP no-main-loot raid bonus.
		if IsTokenAward(award.ItemName) || award.ItemID == "50274" {
			continue
		}
		name := NormalizeItemName(award.ItemName)
		if name == "schattenfrostsplitter" || name == "seelensplitter" || name == "shadowfrost shard" {
			continue
		}
		return true, nil
	}
	return false, nil
}

// Kills binds the kill repository to the same transaction as loot.
func (r *Repo) Kills() *bosskill.Repo { return bosskill.NewRepo(r.db) }
