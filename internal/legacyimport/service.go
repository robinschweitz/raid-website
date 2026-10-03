package legacyimport

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/tokens"
	"raid-website/internal/validation"
)

var ErrActorNotFound = errors.New("import actor is not a known player")

type Counts struct {
	Players    int `json:"players"`
	Characters int `json:"characters"`
	Raids      int `json:"raids"`
	Attendance int `json:"attendance"`
	Loot       int `json:"loot"`
	Tokens     int `json:"tokens"`
	Points     int `json:"points"`
}

type Report struct {
	Mode          string   `json:"mode"`
	Source        string   `json:"source"`
	RawHash       string   `json:"raw_hash"`
	RowsRead      int      `json:"rows_read"`
	Players       int      `json:"players"`
	Characters    int      `json:"characters"`
	Raids         int      `json:"raids"`
	Attendance    int      `json:"attendance"`
	Loot          int      `json:"loot"`
	Tokens        int      `json:"tokens"`
	PointsEntries int      `json:"points_entries"`
	Created       Counts   `json:"created"`
	Skipped       bool     `json:"skipped"`
	Warnings      []string `json:"warnings"`
	Errors        []string `json:"errors"`
}

// Service orchestrates legacy sheet import (dry-run and merge).
type Service struct {
	db      *bun.DB
	imports *Repo
	players *player.Repo
	chars   *character.Repo
	raids   *raid.Repo
	att     *attendance.Repo
	loot    *loot.Repo
	points  *enhancepoints.Repo
	tokens  *tokens.Repo
	fetcher Fetcher
}

func NewService(
	db *bun.DB,
	imports *Repo,
	players *player.Repo,
	chars *character.Repo,
	raids *raid.Repo,
	att *attendance.Repo,
	lootRepo *loot.Repo,
	points *enhancepoints.Repo,
	tokenRepo *tokens.Repo,
	fetcher Fetcher,
) *Service {
	return &Service{
		db:      db,
		imports: imports,
		players: players,
		chars:   chars,
		raids:   raids,
		att:     att,
		loot:    lootRepo,
		points:  points,
		tokens:  tokenRepo,
		fetcher: fetcher,
	}
}

func planFromSheet(data SheetData) (Plan, int, error) {
	raidRows, err := ParseRaidArchive(bytes.NewReader(data.Raid))
	if err != nil {
		return Plan{}, 0, fmt.Errorf("parse raid archive: %w", err)
	}
	lootRows, err := ParseLootArchive(bytes.NewReader(data.Loot))
	if err != nil {
		return Plan{}, 0, fmt.Errorf("parse loot archive: %w", err)
	}
	charRows, err := ParseCharaktere(bytes.NewReader(data.Character))
	if err != nil {
		return Plan{}, 0, fmt.Errorf("parse charaktere: %w", err)
	}
	pointsRows, err := ParsePunkte(bytes.NewReader(data.Points))
	if err != nil {
		return Plan{}, 0, fmt.Errorf("parse punkte: %w", err)
	}
	p := BuildPlan(raidRows, lootRows, charRows, pointsRows)
	rows := len(raidRows) + len(lootRows) + len(charRows) + len(pointsRows)
	return p, rows, nil
}

func reportFromPlan(mode, source string, data SheetData, p Plan, rows int) Report {
	tokensPlanned := 0
	for _, l := range p.Loot {
		if l.IsToken {
			tokensPlanned++
		}
	}
	pointsEntries := 0
	for _, pl := range p.Players {
		if pl.HasPoints && pl.Points != 0 {
			pointsEntries++
		}
	}
	return Report{
		Mode:          mode,
		Source:        source,
		RawHash:       data.Hash,
		RowsRead:      rows,
		Players:       len(p.Players),
		Characters:    len(p.Characters),
		Raids:         len(p.Raids),
		Attendance:    countAttendance(p),
		Loot:          len(p.Loot),
		Tokens:        tokensPlanned,
		PointsEntries: pointsEntries,
		Warnings:      p.Warnings,
		Errors:        p.Errors,
	}
}

func countAttendance(p Plan) int {
	n := 0
	for _, r := range p.Raids {
		n += len(r.Entries)
	}
	return n
}

// List returns past imports, newest first.
func (s *Service) List(ctx context.Context) ([]Import, error) {
	return s.imports.List(ctx)
}

// DryRun fetches and plans an import without writing anything.
func (s *Service) DryRun(ctx context.Context, source, sheetID string) (Report, error) {
	data, err := s.fetcher.Fetch(ctx, sheetID)
	if err != nil {
		return Report{}, err
	}
	p, rows, err := planFromSheet(data)
	if err != nil {
		return Report{}, err
	}
	return reportFromPlan("dry_run", source, data, p, rows), nil
}

// Merge fetches and applies an import. It is idempotent for identical source data.
func (s *Service) Merge(ctx context.Context, actorID int64, source, sheetID string) (Report, error) {
	data, err := s.fetcher.Fetch(ctx, sheetID)
	if err != nil {
		return Report{}, err
	}

	if existing, err := s.imports.FindByHash(ctx, source, data.Hash); err == nil && existing != nil {
		p, rows, perr := planFromSheet(data)
		if perr != nil {
			return Report{}, perr
		}
		rep := reportFromPlan("merge", source, data, p, rows)
		rep.Skipped = true
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("identical sheet already merged as import #%d; nothing to do", existing.ID))
		return rep, nil
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Report{}, err
	}

	p, rows, err := planFromSheet(data)
	if err != nil {
		return Report{}, err
	}
	rep := reportFromPlan("merge", source, data, p, rows)

	if _, err := s.players.Get(ctx, actorID); err != nil {
		return Report{}, ErrActorNotFound
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Report{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	players := s.players.WithTx(tx)
	chars := s.chars.WithTx(tx)
	raids := s.raids.WithTx(tx)
	att := s.att.WithTx(tx)
	lootRepo := s.loot.WithTx(tx)
	pointsRepo := s.points.WithTx(tx)
	tokenRepo := s.tokens.WithTx(tx)
	imports := s.imports.WithTx(tx)

	playerIDs, err := mergePlayers(ctx, players, pointsRepo, actorID, p, now, &rep)
	if err != nil {
		return Report{}, err
	}
	charIDs, err := mergeCharacters(ctx, chars, playerIDs, p, now, &rep)
	if err != nil {
		return Report{}, err
	}
	raidIDs, err := mergeRaids(ctx, raids, att, charIDs, playerIDs, actorID, p, now, &rep)
	if err != nil {
		return Report{}, err
	}
	if err := mergeLoot(ctx, lootRepo, tokenRepo, raidIDs, playerIDs, actorID, p, now, &rep); err != nil {
		return Report{}, err
	}

	started := now
	completed := now
	imp := &Import{
		ActorID:        actorID,
		Source:         source,
		SourceURL:      sheetID,
		RawHash:        data.Hash,
		DryRunReport:   "",
		Status:         "MERGED",
		WarningsCount:  len(rep.Warnings),
		ErrorsCount:    len(rep.Errors),
		CreatedCount:   rep.Created.Players + rep.Created.Characters + rep.Created.Raids + rep.Created.Attendance + rep.Created.Loot + rep.Created.Tokens + rep.Created.Points,
		UpdatedCount:   0,
		UnchangedCount: 0,
		StartedAt:      &started,
		CompletedAt:    &completed,
	}
	if err := imports.Insert(ctx, imp); err != nil {
		return Report{}, fmt.Errorf("record import: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Report{}, fmt.Errorf("commit import: %w", err)
	}
	return rep, nil
}

func mergePlayers(ctx context.Context, players *player.Repo, points *enhancepoints.Repo, actorID int64, p Plan, now time.Time, rep *Report) (map[string]int64, error) {
	ids := make(map[string]int64, len(p.Players))
	for _, pl := range p.Players {
		existing, err := players.FindByName(ctx, pl.Name)
		if err == nil {
			ids[pl.Name] = existing.ID
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("find player %q: %w", pl.Name, err)
		}
		prow := &player.Player{Name: pl.Name, Active: pl.Active, CreatedAt: now, UpdatedAt: now}
		if err := players.Create(ctx, prow); err != nil {
			return nil, fmt.Errorf("create player %q: %w", pl.Name, err)
		}
		ids[pl.Name] = prow.ID
		rep.Created.Players++
	}

	for _, pl := range p.Players {
		if !pl.HasPoints || pl.Points == 0 {
			continue
		}
		id := ids[pl.Name]
		has, err := points.HasTransactionType(ctx, id, "legacy_open")
		if err != nil {
			return nil, fmt.Errorf("check opening balance for %q: %w", pl.Name, err)
		}
		if has {
			continue
		}
		txn := &enhancepoints.Transaction{
			PlayerID:  id,
			Amount:    pl.Points,
			Type:      "legacy_open",
			Reason:    "legacy import opening balance",
			RefType:   "legacy_import",
			ActorID:   actorID,
			CreatedAt: now,
		}
		if err := points.InsertTransaction(ctx, txn); err != nil {
			return nil, fmt.Errorf("opening balance for %q: %w", pl.Name, err)
		}
		rep.Created.Points++
	}
	return ids, nil
}

func mergeCharacters(ctx context.Context, chars *character.Repo, playerIDs map[string]int64, p Plan, now time.Time, rep *Report) (map[string]int64, error) {
	ids := make(map[string]int64, len(p.Characters))
	for _, c := range p.Characters {
		existing, err := chars.FindByName(ctx, c.Name)
		if err == nil {
			ids[c.Name] = existing.ID
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("find character %q: %w", c.Name, err)
		}
		pid := playerIDs[c.PlayerName]
		if pid == 0 {
			return nil, fmt.Errorf("character %q references unknown player %q", c.Name, c.PlayerName)
		}
		row := &character.Character{
			PlayerID:   pid,
			Name:       c.Name,
			Class:      c.Class,
			MainSpec:   c.MainSpec,
			SecondSpec: c.SecondSpec,
			Active:     true,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := chars.Create(ctx, row); err != nil {
			return nil, fmt.Errorf("create character %q: %w", c.Name, err)
		}
		ids[c.Name] = row.ID
		rep.Created.Characters++
	}
	return ids, nil
}

func mergeRaids(ctx context.Context, raids *raid.Repo, att *attendance.Repo, charIDs, playerIDs map[string]int64, actorID int64, p Plan, now time.Time, rep *Report) (map[string]int64, error) {
	ids := make(map[string]int64, len(p.Raids))
	for _, rp := range p.Raids {
		date := now
		if rp.Date != nil {
			date = *rp.Date
		}
		existing, err := raids.FindByName(ctx, rp.Key)
		if err == nil {
			ids[rp.Key] = existing.ID
		} else if errors.Is(err, sql.ErrNoRows) {
			row := &raid.Raid{
				Name:           rp.Key,
				RaidLeaderID:   actorID,
				Status:         validation.RaidStatusCompleted,
				ScheduledStart: &date,
				CompletedAt:    &date,
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if err := raids.Create(ctx, row); err != nil {
				return nil, fmt.Errorf("create raid %q: %w", rp.Key, err)
			}
			ids[rp.Key] = row.ID
			rep.Created.Raids++
		} else {
			return nil, fmt.Errorf("find raid %q: %w", rp.Key, err)
		}

		raidID := ids[rp.Key]
		for _, e := range rp.Entries {
			cid := charIDs[e.Character]
			pid := playerIDs[e.Player]
			if cid == 0 || pid == 0 {
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("raid %q: skipping attendance for %q (missing player/character)", rp.Key, e.Character))
				continue
			}
			exists, err := att.Exists(ctx, raidID, pid, cid)
			if err != nil {
				return nil, fmt.Errorf("check attendance raid %q: %w", rp.Key, err)
			}
			if exists {
				continue
			}
			row := &attendance.Attendance{
				RaidID:      raidID,
				PlayerID:    pid,
				CharacterID: cid,
				Role:        e.Role,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := att.Create(ctx, row); err != nil {
				return nil, fmt.Errorf("create attendance raid %q character %q: %w", rp.Key, e.Character, err)
			}
			rep.Created.Attendance++
		}
	}
	return ids, nil
}

func mergeLoot(ctx context.Context, lootRepo *loot.Repo, tokenRepo *tokens.Repo, raidIDs, playerIDs map[string]int64, actorID int64, p Plan, now time.Time, rep *Report) error {
	for _, l := range p.Loot {
		raidID := raidIDs[l.RaidKey]
		pid := playerIDs[l.PlayerName]
		if raidID == 0 || pid == 0 {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("skipping loot %q for %q (missing raid/player)", l.Item, l.PlayerName))
			continue
		}
		award := &loot.Award{
			RaidID:            raidID,
			ItemID:            l.ItemID,
			ItemName:          l.Item,
			AwardedToPlayerID: pid,
			AwardedByPlayerID: actorID,
			Source:            "legacy_import",
			IsMainSpec:        l.IsMainSpec,
			AwardedAt:         now,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := lootRepo.CreateAward(ctx, award); err != nil {
			return fmt.Errorf("create loot %q: %w", l.Item, err)
		}
		rep.Created.Loot++

		if l.IsToken {
			raidIDCopy := raidID
			awardID := award.ID
			txn := &tokens.Transaction{
				PlayerID:    pid,
				RaidID:      &raidIDCopy,
				LootAwardID: &awardID,
				Amount:      1,
				Type:        "award",
				Reason:      l.Item,
				RefType:     "loot_award",
				RefID:       fmt.Sprintf("%d", award.ID),
				ActorID:     actorID,
				CreatedAt:   now,
			}
			if err := tokenRepo.InsertTransaction(ctx, txn); err != nil {
				return fmt.Errorf("derive token for %q: %w", l.Item, err)
			}
			rep.Created.Tokens++
		}
	}
	return nil
}
