// Package websheetsync delivers manual website loot changes to explicitly linked Google Sheets raids.
package websheetsync

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/googlesheets"
	"raid-website/internal/loot"
)

const auditTab = "Website Sync Audit"

const lootArchiveTab = "Loot Archive"

type Sheets interface {
	Values(context.Context, string, string) ([][]string, error)
	UpdateValues(context.Context, string, string, [][]string) error
	AppendRows(context.Context, string, string, [][]string) error
	AddSheet(context.Context, string, string) error
}

type Service struct {
	db     bun.IDB
	sheets Sheets
}

type payload struct {
	RaidKey       string `json:"raid_key"`
	Timestamp     string `json:"timestamp"`
	CharacterName string `json:"character_name"`
	ItemName      string `json:"item_name"`
	Priority      string `json:"priority"`
	ItemID        string `json:"item_id"`
	ImportID      string `json:"import_id"`
	AwardID       int64  `json:"award_id"`
	SyncID        string `json:"sync_id"`
	UpdatedAt     string `json:"updated_at"`
	Status        string `json:"status"`
}

type outbox struct {
	bun.BaseModel `bun:"table:website_loot_delivery_outbox"`

	ID          int64      `bun:"id,pk"`
	OperationID string     `bun:"operation_id"`
	SheetID     string     `bun:"sheet_id"`
	LootAwardID int64      `bun:"loot_award_id"`
	ImportID    string     `bun:"import_id"`
	Operation   string     `bun:"operation_type"`
	PayloadJSON string     `bun:"payload_json"`
	Status      string     `bun:"status"`
	Attempts    int        `bun:"attempt_count"`
	LastError   string     `bun:"attempt_error"`
	CreatedAt   time.Time  `bun:"created_at"`
	SentAt      *time.Time `bun:"sent_at"`
}

func NewService(db bun.IDB, sheets Sheets) *Service { return &Service{db: db, sheets: sheets} }

// EnqueueAward records the exact sheet payload in the caller's transaction.
// Unlinked and non-manual awards intentionally remain local-only.
func (s *Service) EnqueueAward(ctx context.Context, tx bun.IDB, award *loot.Award, actorID int64) error {
	if s == nil || award.Source != "manual" {
		return nil
	}
	var link struct {
		SheetID string `bun:"sheet_id"`
		RaidKey string `bun:"raid_key"`
	}
	err := tx.NewSelect().TableExpr("legacy_sync_raid_mappings").Column("sheet_id", "raid_key").Where("source = ? AND raid_id = ?", "google_sheets", award.RaidID).Scan(ctx, &link)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	name := award.AwardedToCharacterName
	if name == "" && award.AwardedToCharacterID != nil {
		if err := tx.NewSelect().TableExpr("characters").Column("name").Where("id = ?", *award.AwardedToCharacterID).Scan(ctx, &name); err != nil {
			return err
		}
	}
	operationID, err := newID()
	if err != nil {
		return err
	}
	var superseded []int64
	if err := tx.NewSelect().TableExpr("website_loot_delivery_outbox").Column("id").Where("loot_award_id = ? AND status IN ('PENDING', 'FAILED')", award.ID).Scan(ctx, &superseded); err != nil {
		return err
	}
	if len(superseded) > 0 {
		now := time.Now().UTC()
		if _, err := tx.ExecContext(ctx, "UPDATE website_loot_delivery_outbox SET status = 'SUPERSEDED', attempt_error = ? WHERE loot_award_id = ? AND status IN ('PENDING', 'FAILED')", "superseded by "+operationID, award.ID); err != nil {
			return err
		}
		for _, id := range superseded {
			if _, err := tx.ExecContext(ctx, "INSERT INTO website_loot_delivery_events (outbox_id, event_type, detail, created_at) VALUES (?, 'SUPERSEDED', ?, ?)", id, "superseded by "+operationID, now); err != nil {
				return err
			}
		}
	}
	characterName := strings.TrimSpace(name)
	itemName := strings.TrimSpace(award.ItemName)
	raidKey := strings.TrimSpace(link.RaidKey)
	itemID := strings.TrimSpace(award.ItemID)
	importID := fmt.Sprintf("website-award-%d", award.ID)
	timestamp := award.AwardedAt.In(berlinLocation).Format("02.01.2006")
	fingerprint, identity, err := legacyLootIdentity(raidKey, timestamp, characterName, itemName, itemID, string(award.AwardMethod), importID)
	if err != nil {
		return err
	}
	var mappedAwardID int64
	err = tx.NewSelect().TableExpr("legacy_loot_mappings").Column("loot_award_id").
		Where("source = ? AND sheet_id = ? AND tab = ? AND identity = ?", "google_sheets", link.SheetID, lootArchiveTab, identity).
		Scan(ctx, &mappedAwardID)
	if err == nil {
		if mappedAwardID != award.ID {
			return fmt.Errorf("website loot mapping %q belongs to award %d, not %d", importID, mappedAwardID, award.ID)
		}
	} else if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.ExecContext(ctx, "INSERT INTO legacy_loot_mappings (source, sheet_id, tab, identity, external_id, fingerprint, loot_award_id) VALUES (?, ?, ?, ?, ?, ?, ?)", "google_sheets", link.SheetID, lootArchiveTab, identity, importID, fingerprint, award.ID); err != nil {
			return err
		}
	} else {
		return err
	}
	status, operation := "ACTIVE", "UPSERT"
	if award.WithdrawnAt != nil {
		status, operation = "WITHDRAWN", "WITHDRAW"
	}
	p := payload{RaidKey: raidKey, Timestamp: timestamp, CharacterName: characterName, ItemName: itemName, Priority: string(award.AwardMethod), ItemID: itemID, ImportID: importID, AwardID: award.ID, SyncID: operationID, UpdatedAt: award.UpdatedAt.UTC().Format(time.RFC3339), Status: status}
	encoded, err := json.Marshal(p)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, "INSERT INTO website_loot_delivery_outbox (operation_id, source, sheet_id, raid_key, loot_award_id, import_id, operation_type, payload_json, status, created_by_player_id, created_at) VALUES (?, 'google_sheets', ?, ?, ?, ?, ?, ?, 'PENDING', ?, ?)", operationID, link.SheetID, raidKey, award.ID, importID, operation, string(encoded), actorID, now)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO website_loot_delivery_events (outbox_id, event_type, created_at) VALUES (?, 'ENQUEUED', ?)", id, now)
	return err
}

var berlinLocation = func() *time.Location {
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		panic(fmt.Sprintf("load Europe/Berlin timezone: %v", err))
	}
	return location
}()

// legacyLootIdentity duplicates the stable legacy-import provenance contract.
// Keep it here to avoid a package dependency cycle with legacyimport.
func legacyLootIdentity(raidKey, timestamp, characterName, itemName, itemID, priority, externalID string) (fingerprint, identity string, err error) {
	content, err := json.Marshal([]string{raidKey, timestamp, characterName, itemName, itemID, priority})
	if err != nil {
		return "", "", err
	}
	fingerprint = fmt.Sprintf("%x", sha256.Sum256(content))
	identity = fmt.Sprintf("id:%x", sha256.Sum256([]byte(externalID)))
	return fingerprint, identity, nil
}

// Flush retries every unsent delivery. It intentionally leaves failures durable
// so an administrator can retry them after repairing sheet permissions/configuration.
type FlushResult struct {
	Sent   int `json:"sent"`
	Failed int `json:"failed"`
}

func (s *Service) Flush(ctx context.Context) (FlushResult, error) {
	if s == nil || s.sheets == nil {
		return FlushResult{}, errors.New("Google Sheets delivery is not configured")
	}
	var rows []outbox
	if err := s.db.NewSelect().Model(&rows).Where("status IN ('PENDING', 'FAILED')").Order("id ASC").Scan(ctx); err != nil {
		return FlushResult{}, err
	}
	result := FlushResult{}
	for i := range rows {
		if err := s.deliver(ctx, &rows[i]); err != nil {
			if markErr := s.mark(ctx, rows[i].ID, "FAILED", err.Error()); markErr != nil {
				return result, markErr
			}
			result.Failed++
			continue
		}
		if err := s.mark(ctx, rows[i].ID, "SENT", ""); err != nil {
			return result, err
		}
		result.Sent++
	}
	return result, nil
}

func (s *Service) deliver(ctx context.Context, row *outbox) error {
	var p payload
	if err := json.Unmarshal([]byte(row.PayloadJSON), &p); err != nil {
		return fmt.Errorf("decode delivery payload: %w", err)
	}
	values, err := s.sheets.Values(ctx, row.SheetID, "'Loot Archive'!A:K")
	if err != nil {
		return err
	}
	line := []string{p.RaidKey, p.Timestamp, p.CharacterName, p.ItemName, p.Priority, p.ItemID, p.ImportID, fmt.Sprint(p.AwardID), p.SyncID, p.UpdatedAt, p.Status}
	rowNumber := 0
	for i := 1; i < len(values); i++ {
		if len(values[i]) >= 7 && strings.TrimSpace(values[i][6]) == p.ImportID {
			rowNumber = i + 1
			break
		}
	}
	if rowNumber == 0 {
		if err := s.sheets.AppendRows(ctx, row.SheetID, "'Loot Archive'!A:K", [][]string{line}); err != nil {
			return err
		}
	} else if err := s.sheets.UpdateValues(ctx, row.SheetID, fmt.Sprintf("'Loot Archive'!A%d:K%d", rowNumber, rowNumber), [][]string{line}); err != nil {
		return err
	}
	if err := s.sheets.AddSheet(ctx, row.SheetID, auditTab); err != nil {
		if !errors.Is(err, googlesheets.ErrSheetExists) {
			return fmt.Errorf("ensure %s tab: %w", auditTab, err)
		}
	} else if err := s.sheets.UpdateValues(ctx, row.SheetID, "'Website Sync Audit'!A1:F1", [][]string{{"Website Sync ID", "Website Award ID", "Action", "Status", "Website Updated At", "Detail"}}); err != nil {
		return fmt.Errorf("write %s header: %w", auditTab, err)
	}
	return s.sheets.AppendRows(ctx, row.SheetID, "'Website Sync Audit'!A:F", [][]string{{row.OperationID, fmt.Sprint(row.LootAwardID), row.Operation, p.Status, p.UpdatedAt, ""}})
}

func (s *Service) mark(ctx context.Context, id int64, status, detail string) error {
	now := time.Now().UTC()
	var sentAt any
	if status == "SENT" {
		sentAt = now
	}
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE website_loot_delivery_outbox SET status = ?, attempt_count = attempt_count + 1, last_attempt_at = ?, attempt_error = ?, sent_at = COALESCE(?, sent_at) WHERE id = ?", status, now, detail, sentAt, id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO website_loot_delivery_events (outbox_id, event_type, detail, created_at) VALUES (?, ?, ?, ?)", id, status, detail, now)
		return err
	})
}

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate sync operation ID: %w", err)
	}
	return "website-sync-" + hex.EncodeToString(b), nil
}
