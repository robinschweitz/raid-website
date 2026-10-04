package legacyimport

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/raidcorrection"
	"raid-website/internal/tokens"
	"raid-website/internal/validation"
)

var ErrActorNotFound = errors.New("import actor is not a known player")
var ErrPreviewRequired = errors.New("import preview required")
var ErrSourceChanged = errors.New("import source changed since preview")

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
	imports     *Repo
	players     *player.Repo
	chars       *character.Repo
	raids       *raid.Repo
	att         *attendance.Repo
	loot        *loot.Repo
	points      *enhancepoints.Repo
	tokens      *tokens.Repo
	fetcher     Fetcher
	corrections *raidcorrection.Service
}

// NewService constructs an importer with explicit repositories and source fetching.
func NewService(imports *Repo, players *player.Repo, chars *character.Repo, raids *raid.Repo, att *attendance.Repo, lootRepo *loot.Repo, points *enhancepoints.Repo, tokenRepo *tokens.Repo, fetcher Fetcher) *Service {
	return &Service{imports: imports, players: players, chars: chars, raids: raids, att: att, loot: lootRepo, points: points, tokens: tokenRepo, fetcher: fetcher}
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

// DryRun fetches and reviews an import, persisting only successful preview metadata.
// It never writes domain records.
func (s *Service) DryRun(ctx context.Context, source, sheetID string) (Report, error) {
	data, err := s.fetcher.Fetch(ctx, sheetID)
	if err != nil {
		return Report{}, err
	}
	p, rows, err := planFromSheet(data)
	if err != nil {
		return Report{}, err
	}
	rep := reportFromPlan("dry_run", source, data, p, rows)
	err = s.transaction(ctx, true, func(bound *Service) error {
		_, err := bound.review(ctx, source, sheetID, p, &rep)
		if err != nil {
			return err
		}
		if len(rep.Errors) > 0 {
			return nil
		}
		return bound.imports.recordPreview(ctx, source, sheetID, data.Hash)
	})
	return rep, err
}

// Merge fetches and applies an import. It is idempotent for identical source data.
func (s *Service) Merge(ctx context.Context, actorID int64, source, sheetID, expectedHash, reason string) (Report, error) {
	data, err := s.fetcher.Fetch(ctx, sheetID)
	if err != nil {
		return Report{}, err
	}

	rep := Report{Mode: "merge", Source: source, RawHash: data.Hash, Warnings: []string{}, Errors: []string{}}

	err = s.transaction(ctx, true, func(s *Service) error {
		if existing, err := s.imports.FindByHash(ctx, source, sheetID, data.Hash); err == nil {
			if existing.DryRunReport != "" {
				if err := json.Unmarshal([]byte(existing.DryRunReport), &rep); err != nil {
					return fmt.Errorf("decode saved report: %w", err)
				}
			} else {
				rep.Warnings = append(rep.Warnings, "legacy provenance missing: original report unavailable; no created counts reconstructed")
			}
			rep.Skipped = true
			return nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if expectedHash == "" {
			return ErrPreviewRequired
		}
		if expectedHash != data.Hash {
			return ErrSourceChanged
		}
		hasPreview, err := s.imports.hasPreview(ctx, source, sheetID, data.Hash)
		if err != nil {
			return err
		}
		if !hasPreview {
			return ErrPreviewRequired
		}
		p, rows, err := planFromSheet(data)
		if err != nil {
			return err
		}
		rep = reportFromPlan("merge", source, data, p, rows)
		if _, err := s.players.Get(ctx, actorID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrActorNotFound
			}
			return err
		}
		p, err = s.review(ctx, source, sheetID, p, &rep)
		if err != nil {
			return err
		}
		if p.RequiresReason && strings.TrimSpace(reason) == "" {
			rep.Errors = append(rep.Errors, "reason_required: additions to existing imported raids require an explicit correction reason")
		}
		if len(rep.Errors) > 0 {
			rep.Created = Counts{}
			return nil
		}
		rep.Created = Counts{}
		pointsBefore, err := s.imports.pointTransactionCount(ctx)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		playerIDs, err := mergePlayers(ctx, s.players, s.points, actorID, p, now, &rep)
		if err != nil {
			return err
		}
		charIDs, err := s.mergeCharacters(ctx, playerIDs, p, now, &rep)
		if err != nil {
			return err
		}
		raidIDs, err := s.mergeRaids(ctx, source, sheetID, reason, charIDs, playerIDs, actorID, p, now, &rep)
		if err != nil {
			return err
		}
		if err := s.mergeLoot(ctx, source, sheetID, reason, raidIDs, playerIDs, actorID, p, now, &rep); err != nil {
			return err
		}
		pointsAfter, err := s.imports.pointTransactionCount(ctx)
		if err != nil {
			return err
		}
		rep.Created.Points = pointsAfter - pointsBefore
		saved, err := json.Marshal(rep)
		if err != nil {
			return err
		}

		started := now
		completed := now
		imp := &Import{
			ActorID:        actorID,
			Source:         source,
			SourceURL:      sheetID,
			RawHash:        data.Hash,
			DryRunReport:   string(saved),
			Status:         "MERGED",
			WarningsCount:  len(rep.Warnings),
			ErrorsCount:    len(rep.Errors),
			CreatedCount:   rep.Created.Players + rep.Created.Characters + rep.Created.Raids + rep.Created.Attendance + rep.Created.Loot + rep.Created.Tokens + rep.Created.Points,
			UpdatedCount:   0,
			UnchangedCount: 0,
			StartedAt:      &started,
			CompletedAt:    &completed,
			Notes:          strings.TrimSpace(reason),
		}
		if err := s.imports.Insert(ctx, imp); err != nil {
			return fmt.Errorf("record import: %w", err)
		}
		return nil
	})
	return rep, err
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
		prow := &player.Player{Name: pl.Name, Active: pl.Active, Role: validation.RoleGuest, CreatedAt: now, UpdatedAt: now}
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

func (s *Service) mergeCharacters(ctx context.Context, playerIDs map[string]int64, p Plan, now time.Time, rep *Report) (map[string]int64, error) {
	ids := make(map[string]int64, len(p.Characters))
	for _, c := range p.Characters {
		matches, err := s.imports.charactersByName(ctx, c.Name)
		if err != nil {
			return nil, fmt.Errorf("find character %q: %w", c.Name, err)
		}
		pid := playerIDs[c.PlayerName]
		for _, existing := range matches {
			if existing.PlayerID == pid {
				ids[c.Name] = existing.ID
			}
		}
		if ids[c.Name] != 0 {
			continue
		}
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
		if err := s.chars.Create(ctx, row); err != nil {
			return nil, fmt.Errorf("create character %q: %w", c.Name, err)
		}
		ids[c.Name] = row.ID
		rep.Created.Characters++
	}
	return ids, nil
}

func (s *Service) mergeRaids(ctx context.Context, source, sheetID, reason string, charIDs, playerIDs map[string]int64, actorID int64, p Plan, now time.Time, rep *Report) (map[string]int64, error) {
	ids := make(map[string]int64, len(p.Raids))
	for _, rp := range p.Raids {
		date := now
		if rp.Date != nil {
			date = *rp.Date
		}
		if rp.ExistingID != 0 {
			ids[rp.Key] = rp.ExistingID
		} else {
			row := &raid.Raid{
				Name:           rp.Key,
				RaidLeaderID:   actorID,
				Status:         validation.RaidStatusCompleted,
				ScheduledStart: &date,
				CompletedAt:    &date,
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if err := s.raids.Create(ctx, row); err != nil {
				return nil, fmt.Errorf("create raid %q: %w", rp.Key, err)
			}
			ids[rp.Key] = row.ID
			if err := s.imports.insertRaidMapping(ctx, &raidMapping{Source: source, SheetID: sheetID, RaidKey: rp.Key, RaidID: row.ID}); err != nil {
				return nil, err
			}
			rep.Created.Raids++
		}

		raidID := ids[rp.Key]
		for _, e := range rp.Entries {
			cid := charIDs[e.Character]
			pid := playerIDs[e.Player]
			if cid == 0 || pid == 0 {
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("raid %q: skipping attendance for %q (missing player/character)", rp.Key, e.Character))
				continue
			}
			exists, err := s.att.Exists(ctx, raidID, pid, cid)
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
			if rp.ExistingID != 0 {
				err = s.corrections.SaveImportedAttendance(ctx, row, actorID, reason)
			} else {
				err = s.att.Create(ctx, row)
			}
			if err != nil {
				return nil, fmt.Errorf("create attendance raid %q character %q: %w", rp.Key, e.Character, err)
			}
			rep.Created.Attendance++
		}
	}
	return ids, nil
}

func (s *Service) mergeLoot(ctx context.Context, source, sheetID, reason string, raidIDs, playerIDs map[string]int64, actorID int64, p Plan, now time.Time, rep *Report) error {
	existingRaids := map[string]bool{}
	for _, rp := range p.Raids {
		existingRaids[rp.Key] = rp.ExistingID != 0
	}
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
		if l.Date != nil {
			award.AwardedAt = *l.Date
		}
		var err error
		if existingRaids[l.RaidKey] {
			err = s.corrections.SaveImportedLoot(ctx, award, actorID, reason)
		} else {
			err = s.loot.CreateAward(ctx, award)
		}
		if err != nil {
			return fmt.Errorf("create loot %q: %w", l.Item, err)
		}
		rep.Created.Loot++
		if err := s.imports.insertMapping(ctx, &lootMapping{Source: source, SheetID: sheetID, Tab: "Loot Archive", Identity: l.Identity, ExternalID: l.ExternalID, Fingerprint: l.Fingerprint, LootAwardID: award.ID}); err != nil {
			return err
		}

		if l.IsToken && existingRaids[l.RaidKey] {
			rep.Created.Tokens++
		} else if l.IsToken {
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
			if err := s.tokens.InsertTransaction(ctx, txn); err != nil {
				return fmt.Errorf("derive token for %q: %w", l.Item, err)
			}
			rep.Created.Tokens++
		}
	}
	return nil
}
