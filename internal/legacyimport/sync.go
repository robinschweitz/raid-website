package legacyimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DryRunSync previews a linked-sheet synchronization. It never bootstraps records.
func (s *Service) DryRunSync(ctx context.Context, source, sheetID string) (Report, error) {
	data, err := s.fetcher.Fetch(ctx, sheetID)
	if err != nil {
		return Report{}, err
	}
	var rep Report
	err = s.transaction(ctx, true, func(bound *Service) error {
		p, rows, err := planFromSheet(data)
		if err != nil {
			return err
		}
		rep = reportFromPlan("dry_run", source, data, p, rows)
		p, err = bound.review(ctx, source, sheetID, p, &rep, true)
		if err != nil {
			return err
		}
		if err := bound.validateSync(ctx, source, sheetID, p, &rep); err != nil {
			return err
		}
		if _, err := bound.syncFinalizations(ctx, source, sheetID, p, data.Setup, &rep); err != nil {
			return err
		}
		if len(rep.Errors) == 0 {
			return bound.imports.recordPreview(ctx, source, sheetID, data.Hash)
		}
		return nil
	})
	return rep, err
}

// MergeSync applies only changes to explicitly linked raids. Fetching, review, and
// mutation share one transaction so a changed snapshot cannot bypass the review.
func (s *Service) MergeSync(ctx context.Context, actorID int64, source, sheetID, expectedHash, reason string) (Report, error) {
	if expectedHash == "" {
		return Report{}, ErrPreviewRequired
	}
	var rep Report
	var finalizations []syncFinalization
	err := s.transaction(ctx, true, func(bound *Service) error {
		data, err := bound.fetcher.Fetch(ctx, sheetID)
		if err != nil {
			return err
		}
		if data.Hash != expectedHash {
			return ErrSourceChanged
		}
		p, rows, err := planFromSheet(data)
		if err != nil {
			return err
		}
		rep = reportFromPlan("sync", source, data, p, rows)
		p, err = bound.review(ctx, source, sheetID, p, &rep, true)
		if err != nil {
			return err
		}
		if err := bound.validateSync(ctx, source, sheetID, p, &rep); err != nil {
			return err
		}
		finalizations, err = bound.syncFinalizations(ctx, source, sheetID, p, data.Setup, &rep)
		if err != nil {
			return err
		}
		if p.RequiresReason && strings.TrimSpace(reason) == "" {
			rep.Errors = append(rep.Errors, "reason_required: additions to existing imported raids require an explicit correction reason")
			rep.Created = Counts{}
			rep.ArchivedRaids = 0
		}
		if len(rep.Errors) > 0 {
			return nil
		}
		if _, err := bound.players.Get(ctx, actorID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrActorNotFound
			}
			return err
		}
		hasPreview, err := bound.imports.hasPreview(ctx, source, sheetID, data.Hash)
		if err != nil {
			return err
		}
		if !hasPreview {
			return ErrPreviewRequired
		}
		if existing, err := bound.imports.FindByHash(ctx, source, sheetID, data.Hash); err == nil {
			if err := json.Unmarshal([]byte(existing.DryRunReport), &rep); err != nil {
				return fmt.Errorf("decode saved report: %w", err)
			}
			rep.Skipped = true
			return nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		playerIDs, err := bound.syncPlayerIDs(ctx, source, sheetID, p)
		if err != nil {
			return err
		}
		characterIDs, err := bound.syncCharacterIDs(ctx, source, sheetID, p, playerIDs)
		if err != nil {
			return err
		}
		raidIDs := make(map[string]int64, len(p.Raids))
		for _, rp := range p.Raids {
			raidIDs[rp.Key] = rp.ExistingID
		}
		rep.Created = Counts{}
		now := time.Now().UTC()
		if _, err := bound.mergeRaids(ctx, source, sheetID, reason, characterIDs, playerIDs, actorID, p, now, &rep); err != nil {
			return err
		}
		if err := bound.mergeLoot(ctx, source, sheetID, data.Hash, reason, raidIDs, playerIDs, characterIDs, actorID, p, now, &rep, true); err != nil {
			return err
		}
		return nil
	})
	if err != nil || len(rep.Errors) > 0 {
		return rep, err
	}
	// Completion starts its own transaction, so it must run only after the sync
	// transaction has committed. The completion service and successor mapping
	// make repeated requests idempotent.
	if err := s.applySyncFinalizations(ctx, actorID, source, sheetID, reason, finalizations, &rep); err != nil {
		return rep, err
	}
	err = s.transaction(ctx, true, func(bound *Service) error {
		saved, err := json.Marshal(rep)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		return bound.imports.Insert(ctx, &Import{ActorID: actorID, Source: source, SourceURL: sheetID, RawHash: rep.RawHash, DryRunReport: string(saved), Status: "MERGED", WarningsCount: len(rep.Warnings), ErrorsCount: len(rep.Errors), CreatedCount: rep.Created.Attendance + rep.Created.Loot + rep.Created.Tokens + rep.Created.Points, UpdatedCount: rep.Updated + rep.Withdrawn + rep.Restored + rep.FinalizedRaids, StartedAt: &now, CompletedAt: &now, Notes: strings.TrimSpace(reason)})
	})
	return rep, err
}

func (s *Service) validateSync(ctx context.Context, source, sheetID string, p Plan, rep *Report) error {
	for _, rp := range p.Raids {
		if rp.ExistingID == 0 {
			rep.Errors = append(rep.Errors, "sync_unmapped_raid: "+rp.Key)
		}
	}
	for _, planned := range p.Players {
		if _, err := s.imports.syncPlayerID(ctx, source, sheetID, planned.Name); errors.Is(err, sql.ErrNoRows) {
			rep.Errors = append(rep.Errors, "sync_unmapped_player: "+planned.Name)
		} else if err != nil {
			return err
		}
	}
	for _, planned := range p.Characters {
		if _, err := s.imports.syncCharacterID(ctx, source, sheetID, planned.Name, planned.PlayerName); errors.Is(err, sql.ErrNoRows) {
			rep.Errors = append(rep.Errors, "sync_unmapped_character: "+planned.Name)
		} else if err != nil {
			return err
		}
	}
	if len(rep.Errors) > 0 {
		rep.Created = Counts{}
		rep.ArchivedRaids = 0
	}
	return nil
}

func (s *Service) syncPlayerIDs(ctx context.Context, source, sheetID string, p Plan) (map[string]int64, error) {
	ids := make(map[string]int64, len(p.Players))
	for _, planned := range p.Players {
		id, err := s.imports.syncPlayerID(ctx, source, sheetID, planned.Name)
		if err != nil {
			return nil, fmt.Errorf("find synchronized player %q: %w", planned.Name, err)
		}
		ids[planned.Name] = id
	}
	return ids, nil
}

func (s *Service) syncCharacterIDs(ctx context.Context, source, sheetID string, p Plan, playerIDs map[string]int64) (map[string]int64, error) {
	ids := make(map[string]int64, len(p.Characters))
	for _, planned := range p.Characters {
		id, err := s.imports.syncCharacterID(ctx, source, sheetID, planned.Name, planned.PlayerName)
		if err != nil {
			return nil, fmt.Errorf("find synchronized character %q: %w", planned.Name, err)
		}
		if playerIDs[planned.PlayerName] == 0 {
			return nil, fmt.Errorf("synchronized character %q has unmapped owner", planned.Name)
		}
		ids[planned.Name] = id
	}
	return ids, nil
}
