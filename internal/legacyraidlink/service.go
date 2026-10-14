package legacyraidlink

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
	"raid-website/internal/player"
	"raid-website/internal/raid"
)

const GoogleSheetsSource = "google_sheets"

var (
	ErrNotFound        = errors.New("legacy raid link not found")
	ErrSourceInvalid   = errors.New("legacy raid link source must be google_sheets")
	ErrSheetIDRequired = errors.New("legacy raid link sheet ID is required")
	ErrRaidKeyRequired = errors.New("legacy raid link raid key is required")
	ErrReasonRequired  = errors.New("legacy raid link reason is required")
	ErrAlreadyLinked   = errors.New("legacy raid is already linked")
	ErrActorNotFound   = errors.New("legacy raid link actor is not a known player")
)

type Link struct {
	Source           string    `json:"source"`
	SheetID          string    `json:"sheet_id"`
	RaidKey          string    `json:"raid_key"`
	RaidID           int64     `json:"raid_id"`
	LinkedByPlayerID int64     `json:"linked_by_player_id"`
	LinkedAt         time.Time `json:"linked_at"`
	Reason           string    `json:"reason"`
}

type CreateInput struct {
	Source  string
	SheetID string
	RaidKey string
	Reason  string
}

type Service struct {
	db bun.IDB
}

func NewService(db bun.IDB) *Service {
	return &Service{db: db}
}

func (s *Service) Get(ctx context.Context, raidID int64) (*Link, error) {
	link := new(Link)
	err := s.db.NewSelect().
		TableExpr("legacy_raid_mappings AS mapping").
		ColumnExpr("mapping.source, mapping.sheet_id, mapping.raid_key, mapping.raid_id, metadata.linked_by_player_id, metadata.linked_at, metadata.link_reason AS reason").
		Join("JOIN legacy_raid_sync_links AS metadata ON metadata.source = mapping.source AND metadata.sheet_id = mapping.sheet_id AND metadata.raid_key = mapping.raid_key").
		Where("mapping.raid_id = ?", raidID).
		Scan(ctx, link)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return link, nil
}

func (s *Service) Create(ctx context.Context, raidID, actorID int64, input CreateInput) (*Link, error) {
	input.Source = strings.TrimSpace(input.Source)
	input.SheetID = strings.TrimSpace(input.SheetID)
	input.RaidKey = strings.TrimSpace(input.RaidKey)
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Source != GoogleSheetsSource {
		return nil, ErrSourceInvalid
	}
	if input.SheetID == "" {
		return nil, ErrSheetIDRequired
	}
	if input.RaidKey == "" {
		return nil, ErrRaidKeyRequired
	}
	if input.Reason == "" {
		return nil, ErrReasonRequired
	}

	link := &Link{Source: input.Source, SheetID: input.SheetID, RaidKey: input.RaidKey, RaidID: raidID, LinkedByPlayerID: actorID, LinkedAt: time.Now().UTC().Truncate(time.Microsecond), Reason: input.Reason}
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := player.NewRepo(tx).Get(ctx, actorID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrActorNotFound
			}
			return err
		}
		if _, err := raid.NewRepo(tx).Get(ctx, raidID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO legacy_raid_mappings (source, sheet_id, raid_key, raid_id) VALUES (?, ?, ?, ?)", link.Source, link.SheetID, link.RaidKey, link.RaidID); err != nil {
			return mapConflict(err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO legacy_raid_sync_links (source, sheet_id, raid_key, linked_by_player_id, linked_at, link_reason) VALUES (?, ?, ?, ?, ?, ?)", link.Source, link.SheetID, link.RaidKey, link.LinkedByPlayerID, link.LinkedAt, link.Reason); err != nil {
			return err
		}
		after, err := json.Marshal(link)
		if err != nil {
			return err
		}
		return audit.NewRepo(tx).Insert(ctx, &audit.Log{ActorID: &actorID, EntityType: "legacy_raid_link", EntityID: strconv.FormatInt(raidID, 10), Action: "link", BeforeJSON: "null", AfterJSON: string(after), Reason: link.Reason, CreatedAt: link.LinkedAt})
	})
	if err != nil {
		return nil, err
	}
	return link, nil
}

func mapConflict(err error) error {
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return errors.Join(ErrAlreadyLinked, err)
	}
	return err
}
