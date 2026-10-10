package legacyimport

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/logger"
	"raid-website/internal/loot"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/raidcorrection"
	"raid-website/internal/raidseries"
	"raid-website/internal/registration"
	"raid-website/internal/validation"
)

var ErrActorNotFound = errors.New("import actor is not a known player")
var ErrPreviewRequired = errors.New("import preview required")
var ErrSourceChanged = errors.New("import source changed since preview")

type Counts struct {
	Series     int `json:"series"`
	Players    int `json:"players"`
	Characters int `json:"characters"`
	Raids      int `json:"raids"`
	Attendance int `json:"attendance"`
	Loot       int `json:"loot"`
	Tokens     int `json:"tokens"`
	Points     int `json:"points"`
}

type Report struct {
	ArchivedRaids int          `json:"archived_raids"`
	Setup         *SetupReport `json:"setup,omitempty"`
	Mode          string       `json:"mode"`
	Source        string       `json:"source"`
	RawHash       string       `json:"raw_hash"`
	RowsRead      int          `json:"rows_read"`
	Players       int          `json:"players"`
	Characters    int          `json:"characters"`
	Raids         int          `json:"raids"`
	Attendance    int          `json:"attendance"`
	Loot          int          `json:"loot"`
	Tokens        int          `json:"tokens"`
	PointsEntries int          `json:"points_entries"`
	Created       Counts       `json:"created"`
	Skipped       bool         `json:"skipped"`
	Warnings      []string     `json:"warnings"`
	Errors        []string     `json:"errors"`
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
	series      *raidseries.Repo
	fetcher     Fetcher
	corrections *raidcorrection.Service
	external    registration.ExternalClient
}

type characterLink struct {
	Profile registration.ExternalCharacter
	Raw     string
}

// NewService constructs an importer with explicit repositories and source fetching.
func NewService(imports *Repo, players *player.Repo, chars *character.Repo, raids *raid.Repo, att *attendance.Repo, lootRepo *loot.Repo, points *enhancepoints.Repo, series *raidseries.Repo, fetcher Fetcher) *Service {
	return &Service{imports: imports, players: players, chars: chars, raids: raids, att: att, loot: lootRepo, points: points, series: series, fetcher: fetcher}
}

func (s *Service) WithExternalClient(external registration.ExternalClient) *Service {
	s.external = external
	return s
}

func (s *Service) prepareCharacterLinks(ctx context.Context, source, sheetID string, data SheetData, expectedHash string) (map[string]characterLink, []string) {
	if s.external == nil || expectedHash == "" || setupHash(data.Hash, nil) != expectedHash {
		return nil, nil
	}
	p, _, err := planFromSheet(data)
	if err != nil {
		return nil, nil
	}
	logger.Info("legacy import rising-gods linking started source=%s sheet=%s characters=%d", source, sheetID, len(p.Characters))
	links := make(map[string]characterLink)
	seen := make(map[string]CharacterPlan)
	for _, planned := range p.Characters {
		name := strings.TrimSpace(planned.Name)
		if len([]rune(name)) < 3 {
			continue
		}
		if _, exists := seen[strings.ToLower(name)]; !exists {
			seen[strings.ToLower(name)] = planned
		}
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for _, planned := range seen {
		if ctx.Err() != nil {
			break
		}
		planned := planned
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			link, ok := s.lookupCharacterLink(ctx, planned)
			if !ok {
				return
			}
			mu.Lock()
			links[planned.Name] = link
			mu.Unlock()
		}()
	}
	wg.Wait()
	if ctx.Err() != nil {
		logger.Warn("legacy import rising-gods linking cancelled source=%s sheet=%s linked=%d", source, sheetID, len(links))
		return links, nil
	}
	logger.Info("legacy import rising-gods linking finished source=%s sheet=%s linked=%d", source, sheetID, len(links))
	return links, nil
}

func (s *Service) lookupCharacterLink(ctx context.Context, planned CharacterPlan) (characterLink, bool) {
	name := strings.TrimSpace(planned.Name)
	if len([]rune(name)) < 3 {
		return characterLink{}, false
	}
	results, err := s.external.SearchCharacters(ctx, name)
	if err != nil {
		return characterLink{}, false
	}
	exactMatches := make([]registration.ExternalCharacter, 0, 1)
	for _, result := range results {
		if strings.EqualFold(strings.TrimSpace(result.Name), name) {
			exactMatches = append(exactMatches, result)
		}
	}
	if len(exactMatches) != 1 {
		return characterLink{}, false
	}
	profile := exactMatches[0]
	if profile.ProfileID == "" || profile.Name == "" || profile.Class == "" {
		return characterLink{}, false
	}
	if planned.Class != "" && validation.NormalizeClassName(planned.Class) != validation.NormalizeClassName(profile.Class) {
		return characterLink{}, false
	}
	return characterLink{Profile: profile}, true
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
	return s.DryRunWithSetup(ctx, source, sheetID, nil)
}

func (s *Service) DryRunWithSetup(ctx context.Context, source, sheetID string, config *SetupConfig) (Report, error) {
	started := time.Now()
	logger.Info("legacy import dry-run started source=%s sheet=%s setup=%t", source, sheetID, config != nil)
	defer func() {
		logger.Info("legacy import dry-run finished source=%s sheet=%s duration=%s", source, sheetID, time.Since(started))
	}()
	logger.Info("legacy import dry-run fetching source=%s sheet=%s", source, sheetID)
	data, err := s.fetcher.Fetch(ctx, sheetID)
	if err != nil {
		logger.Error("legacy import dry-run fetch failed source=%s sheet=%s err=%v", source, sheetID, err)
		return Report{}, err
	}
	data.Hash = setupHash(data.Hash, config)
	var rep Report
	err = s.transaction(ctx, true, func(bound *Service) error {
		logger.Info("legacy import dry-run planning source=%s sheet=%s hash=%s", source, sheetID, data.Hash)
		p, rows, setupReport, err := bound.planWithSetup(ctx, source, sheetID, data, config)
		if err != nil {
			return err
		}
		rep = reportFromPlan("dry_run", source, data, p, rows)
		rep.Setup = setupReport
		logger.Info("legacy import dry-run reviewing source=%s sheet=%s rows=%d players=%d characters=%d raids=%d loot=%d", source, sheetID, rows, rep.Players, rep.Characters, rep.Raids, rep.Loot)
		_, err = bound.review(ctx, source, sheetID, p, &rep)
		if err != nil {
			return err
		}
		if len(rep.Errors) > 0 {
			logger.Warn("legacy import dry-run completed with errors source=%s sheet=%s errors=%d warnings=%d", source, sheetID, len(rep.Errors), len(rep.Warnings))
			return nil
		}
		logger.Info("legacy import dry-run recording preview source=%s sheet=%s hash=%s warnings=%d", source, sheetID, data.Hash, len(rep.Warnings))
		return bound.imports.recordPreview(ctx, source, sheetID, data.Hash)
	})
	return rep, err
}

// Merge fetches and applies an import. It is idempotent for identical source data.
func (s *Service) Merge(ctx context.Context, actorID int64, source, sheetID, expectedHash, reason string) (Report, error) {
	return s.MergeWithSetup(ctx, actorID, source, sheetID, expectedHash, reason, nil)
}

func (s *Service) MergeWithSetup(ctx context.Context, actorID int64, source, sheetID, expectedHash, reason string, config *SetupConfig) (Report, error) {
	started := time.Now()
	logger.Info("legacy import merge started source=%s sheet=%s actor=%d setup=%t", source, sheetID, actorID, config != nil)
	defer func() {
		logger.Info("legacy import merge finished source=%s sheet=%s duration=%s", source, sheetID, time.Since(started))
	}()
	logger.Info("legacy import merge fetching source=%s sheet=%s", source, sheetID)
	data, err := s.fetcher.Fetch(ctx, sheetID)
	if err != nil {
		logger.Error("legacy import merge fetch failed source=%s sheet=%s err=%v", source, sheetID, err)
		return Report{}, err
	}

	data.Hash = setupHash(data.Hash, config)
	rep := Report{Mode: "merge", Source: source, RawHash: data.Hash, Warnings: []string{}, Errors: []string{}}
	characterLinks, characterLinkWarnings := s.prepareCharacterLinks(ctx, source, sheetID, data, expectedHash)

	err = s.transaction(ctx, true, func(s *Service) error {
		logger.Info("legacy import merge transaction started source=%s sheet=%s hash=%s", source, sheetID, data.Hash)
		if existing, err := s.imports.FindByHash(ctx, source, sheetID, data.Hash); err == nil {
			if existing.DryRunReport != "" {
				if err := json.Unmarshal([]byte(existing.DryRunReport), &rep); err != nil {
					return fmt.Errorf("decode saved report: %w", err)
				}
				rep.Warnings = withoutStaleRisingGodsTimeout(rep.Warnings)
			} else {
				rep.Warnings = append(rep.Warnings, "legacy provenance missing: original report unavailable; no created counts reconstructed")
			}
			if len(characterLinks) > 0 {
				p, _, planErr := planFromSheet(data)
				if planErr != nil {
					return planErr
				}
				linked, linkErr := s.linkExistingImportedCharacters(ctx, p, characterLinks, time.Now().UTC())
				if linkErr != nil {
					return linkErr
				}
				if linked > 0 {
					rep.Warnings = append(rep.Warnings, fmt.Sprintf("Rising-Gods-Verknuepfung: %d bestehende Charaktere nachtraeglich verknuepft", linked))
				}
				logger.Info("legacy import merge skipped hash linked existing characters source=%s sheet=%s linked=%d", source, sheetID, linked)
			}
			rep.Skipped = true
			logger.Info("legacy import merge skipped existing hash source=%s sheet=%s hash=%s", source, sheetID, data.Hash)
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
		p, rows, setupReport, err := s.planWithSetup(ctx, source, sheetID, data, config)
		if err != nil {
			return err
		}
		rep = reportFromPlan("merge", source, data, p, rows)
		rep.Warnings = append(rep.Warnings, characterLinkWarnings...)
		rep.Setup = setupReport
		logger.Info("legacy import merge planned source=%s sheet=%s rows=%d players=%d characters=%d raids=%d loot=%d", source, sheetID, rows, rep.Players, rep.Characters, rep.Raids, rep.Loot)
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
			rep.ArchivedRaids = 0
			logger.Warn("legacy import merge stopped by report errors source=%s sheet=%s errors=%d warnings=%d", source, sheetID, len(rep.Errors), len(rep.Warnings))
			return nil
		}
		rep.Created = Counts{}
		rep.ArchivedRaids = 0
		pointsBefore, err := s.imports.pointTransactionCount(ctx)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		playerIDs, err := mergePlayers(ctx, s.players, s.points, actorID, p, now, &rep)
		if err != nil {
			return err
		}
		logger.Info("legacy import merge players done source=%s sheet=%s created=%d", source, sheetID, rep.Created.Players)
		charIDs, err := s.mergeCharacters(ctx, playerIDs, p, now, characterLinks, &rep)
		if err != nil {
			return err
		}
		logger.Info("legacy import merge characters done source=%s sheet=%s created=%d linked=%d", source, sheetID, rep.Created.Characters, len(characterLinks))
		raidIDs, err := s.mergeRaids(ctx, source, sheetID, reason, charIDs, playerIDs, actorID, p, now, &rep)
		if err != nil {
			return err
		}
		logger.Info("legacy import merge raids done source=%s sheet=%s created=%d attendance=%d", source, sheetID, rep.Created.Raids, rep.Created.Attendance)
		if err := s.mergeLoot(ctx, source, sheetID, reason, raidIDs, playerIDs, charIDs, actorID, p, now, &rep); err != nil {
			return err
		}
		logger.Info("legacy import merge loot done source=%s sheet=%s created=%d tokens=%d", source, sheetID, rep.Created.Loot, rep.Created.Tokens)
		if err := s.archiveImportedHistory(ctx, actorID, reason, p, now, &rep); err != nil {
			return err
		}
		logger.Info("legacy import merge archive classification done source=%s sheet=%s archived=%d", source, sheetID, rep.ArchivedRaids)
		if err := s.mergeSetup(ctx, source, sheetID, p, charIDs, playerIDs, now, &rep); err != nil {
			return err
		}
		logger.Info("legacy import merge setup done source=%s sheet=%s series=%d", source, sheetID, rep.Created.Series)
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
			CreatedCount:   rep.Created.Series + rep.Created.Players + rep.Created.Characters + rep.Created.Raids + rep.Created.Attendance + rep.Created.Loot + rep.Created.Tokens + rep.Created.Points,
			UpdatedCount:   rep.ArchivedRaids,
			UnchangedCount: 0,
			StartedAt:      &started,
			CompletedAt:    &completed,
			Notes:          strings.TrimSpace(reason),
		}
		if err := s.imports.Insert(ctx, imp); err != nil {
			return fmt.Errorf("record import: %w", err)
		}
		logger.Info("legacy import merge recorded source=%s sheet=%s created_total=%d warnings=%d", source, sheetID, imp.CreatedCount, imp.WarningsCount)
		return nil
	})
	return rep, err
}

func withoutStaleRisingGodsTimeout(warnings []string) []string {
	if len(warnings) == 0 {
		return warnings
	}
	filtered := warnings[:0]
	for _, warning := range warnings {
		if strings.Contains(warning, "Rising-Gods-Verknuepfung: Zeitlimit erreicht") {
			continue
		}
		filtered = append(filtered, warning)
	}
	return filtered
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

func (s *Service) mergeCharacters(ctx context.Context, playerIDs map[string]int64, p Plan, now time.Time, links map[string]characterLink, rep *Report) (map[string]int64, error) {
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
				if err := s.linkImportedCharacter(ctx, &existing, &c, links[c.Name], now); err != nil {
					return nil, err
				}
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
		if err := s.linkImportedCharacter(ctx, row, &c, links[c.Name], now); err != nil {
			return nil, err
		}
		if err := s.chars.Create(ctx, row); err != nil {
			return nil, fmt.Errorf("create character %q: %w", c.Name, err)
		}
		ids[c.Name] = row.ID
		rep.Created.Characters++
	}
	return ids, nil
}

func (s *Service) linkExistingImportedCharacters(ctx context.Context, p Plan, links map[string]characterLink, now time.Time) (int, error) {
	linked := 0
	for _, planned := range p.Characters {
		link := links[planned.Name]
		if link.Profile.ProfileID == "" {
			continue
		}
		matches, err := s.imports.charactersByName(ctx, planned.Name)
		if err != nil {
			return 0, fmt.Errorf("find character %q: %w", planned.Name, err)
		}
		for _, existing := range matches {
			before := existing.ExternalProfileID
			if err := s.linkImportedCharacter(ctx, &existing, &planned, link, now); err != nil {
				return 0, err
			}
			if before == "" && existing.ExternalProfileID != "" {
				linked++
			}
		}
	}
	return linked, nil
}

func (s *Service) linkImportedCharacter(ctx context.Context, row *character.Character, planned *CharacterPlan, link characterLink, now time.Time) error {
	if row.ExternalProfileID != "" || link.Profile.ProfileID == "" {
		return nil
	}
	profile := link.Profile
	if planned != nil && planned.Class != "" && validation.NormalizeClassName(planned.Class) != validation.NormalizeClassName(profile.Class) {
		return nil
	}
	if linked, err := s.chars.FindByExternalProfile(ctx, registration.ExternalSourceRisingGods, profile.ProfileID); err == nil && linked.ID != row.ID {
		return nil
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	row.ExternalSource = registration.ExternalSourceRisingGods
	row.ExternalProfileID = profile.ProfileID
	row.ExternalProfileURL = profile.ProfileURL
	row.ExternalProfileJSON = link.Raw
	row.UpdatedAt = now
	if row.ID != 0 {
		return s.chars.Update(ctx, row)
	}
	return nil
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
				ArchivedAt:     &date,
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if rp.CatalogInstanceID != "" {
				instance, size := rp.CatalogInstanceID, rp.RaidSize
				row.CatalogInstanceID, row.RaidSize = &instance, &size
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
				RaidID:        raidID,
				PlayerID:      pid,
				CharacterID:   cid,
				CharacterName: e.Character,
				Class:         e.Class,
				Spec:          e.Spec,
				Role:          e.Role,
				CreatedAt:     now,
				UpdatedAt:     now,
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

func (s *Service) mergeLoot(ctx context.Context, source, sheetID, reason string, raidIDs, playerIDs, charIDs map[string]int64, actorID int64, p Plan, now time.Time, rep *Report) error {
	existingRaids := map[string]bool{}
	for _, rp := range p.Raids {
		existingRaids[rp.Key] = rp.ExistingID != 0
	}
	for _, l := range p.Loot {
		raidID := raidIDs[l.RaidKey]
		pid := playerIDs[l.PlayerName]
		cid := charIDs[l.CharacterName]
		if raidID == 0 || pid == 0 || cid == 0 {
			return fmt.Errorf("loot %q for character %q references unknown raid/player/character", l.Item, l.CharacterName)
		}
		award := &loot.Award{
			RaidID:                 raidID,
			Boss:                   l.Boss,
			ItemID:                 l.ItemID,
			ItemName:               l.Item,
			AwardedToPlayerID:      pid,
			AwardedToCharacterID:   &cid,
			AwardedToCharacterName: l.CharacterName,
			AwardedByPlayerID:      actorID,
			Source:                 "legacy_import",
			AwardMethod:            l.Method,
			AwardedAt:              now,
			CreatedAt:              now,
			UpdatedAt:              now,
		}
		if l.CatalogBossID != "" {
			bossID := l.CatalogBossID
			award.CatalogBossID = &bossID
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

		if l.IsToken {
			rep.Created.Tokens++
		}
	}
	return nil
}
