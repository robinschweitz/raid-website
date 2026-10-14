package legacyimport

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"raid-website/internal/audit"
	"raid-website/internal/validation"
)

// Previously imported COMPLETED history gets the archive classification only.
// This never calls raid finalization, awards rewards, or schedules a successor.
func (s *Service) archiveImportedHistory(ctx context.Context, actorID int64, reason string, p Plan, now time.Time, rep *Report) error {
	for _, planned := range p.Raids {
		if s.sheetArchives != nil {
			if _, pending := s.sheetArchives.raids[planned.Key]; pending {
				continue
			}
		}
		if planned.ExistingID == 0 {
			continue
		}
		rd, err := s.raids.Get(ctx, planned.ExistingID)
		if err != nil {
			return err
		}
		if rd.Status != validation.RaidStatusCompleted || rd.ArchivedAt != nil {
			continue
		}
		before, err := json.Marshal(rd)
		if err != nil {
			return err
		}
		rd.ArchivedAt, rd.UpdatedAt = &now, now
		if err := s.raids.Update(ctx, rd); err != nil {
			return err
		}
		after, err := json.Marshal(rd)
		if err != nil {
			return err
		}
		if err := s.imports.writeAudit(ctx, &audit.Log{ActorID: &actorID, EntityType: "raid", EntityID: strconv.FormatInt(rd.ID, 10), Action: "import_archive", BeforeJSON: string(before), AfterJSON: string(after), Reason: strings.TrimSpace(reason), CreatedAt: now}); err != nil {
			return err
		}
		rep.ArchivedRaids++
	}
	return nil
}
