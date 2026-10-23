// Package unclaimedloot records loot that went to nobody (ADR 089): items
// disenchanted, kept for the guild bank, or that nobody wanted. They are never
// awards: they count for no player, cost no EP and never touch item locks.
package unclaimedloot

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/lootcatalog"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

// Outcome says what happened to the item.
type Outcome string

const (
	Disenchanted Outcome = "DISENCHANTED"
	Bank         Outcome = "BANK"
	Nobody       Outcome = "NOBODY"
)

// Valid reports whether the outcome is one of the three known ones.
func (o Outcome) Valid() bool { return o == Disenchanted || o == Bank || o == Nobody }

var (
	ErrNotFound       = errors.New("unclaimed loot not found")
	ErrInvalid        = errors.New("unclaimed loot needs an item and a known outcome")
	ErrBoss           = errors.New("boss does not belong to the raid's instance")
	ErrRaidState      = errors.New("raid does not accept unclaimed loot")
	ErrReasonRequired = errors.New("a reason is required for an archived raid")
	ErrAlreadyRemoved = errors.New("unclaimed loot already removed")
)

// Entry is one item that went to nobody.
type Entry struct {
	bun.BaseModel `bun:"table:unclaimed_loot"`

	ID                 int64      `bun:"id,pk,autoincrement" json:"id"`
	RaidID             int64      `bun:"raid_id,notnull" json:"raid_id"`
	CatalogBossID      *string    `bun:"catalog_boss_id" json:"catalog_boss_id"`
	Boss               string     `bun:"boss,notnull" json:"boss"`
	ItemID             string     `bun:"item_id,notnull" json:"item_id"`
	ItemName           string     `bun:"item_name,notnull" json:"item_name"`
	Outcome            Outcome    `bun:"outcome,notnull" json:"outcome"`
	HolderName         string     `bun:"holder_name,notnull" json:"holder_name"`
	Notes              string     `bun:"notes,notnull" json:"notes"`
	Source             string     `bun:"source,notnull" json:"source"`
	AddonLineKey       *string    `bun:"addon_line_key" json:"-"`
	CreatedByPlayerID  int64      `bun:"created_by_player_id,notnull" json:"created_by_player_id"`
	CreatedAt          time.Time  `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt          time.Time  `bun:"updated_at,notnull" json:"updated_at"`
	ArchivedAt         *time.Time `bun:"archived_at,nullzero" json:"archived_at,omitempty"`
	ArchivedByPlayerID *int64     `bun:"archived_by_player_id,nullzero" json:"archived_by_player_id,omitempty"`
	ArchiveReason      string     `bun:"archive_reason,notnull" json:"archive_reason,omitempty"`
}

// Input is what staff (or an addon import) records.
type Input struct {
	CatalogBossID string
	ItemID        string
	ItemName      string
	Outcome       Outcome
	HolderName    string
	Notes         string
	Reason        string // required for archived raids
}

// Service holds the rules.
type Service struct {
	repo    *Repo
	raids   *raid.Repo
	catalog *lootcatalog.Service
}

// NewService wires the service.
func NewService(repo *Repo, raids *raid.Repo, catalog *lootcatalog.Service) *Service {
	return &Service{repo: repo, raids: raids, catalog: catalog}
}

// List returns a raid's unclaimed loot.
func (s *Service) List(ctx context.Context, raidID int64) ([]Entry, error) {
	return s.repo.ListByRaid(ctx, raidID)
}

// Create records an item that went to nobody, entered by staff.
func (s *Service) Create(ctx context.Context, actorID, raidID int64, in Input) (*Entry, error) {
	return s.create(ctx, actorID, raidID, in, "manual", nil)
}

// CreateFromAddon records a disenchanted or banked line of an addon export.
// The line key makes it idempotent: a second import returns the stored entry
// with created = false.
func (s *Service) CreateFromAddon(ctx context.Context, actorID, raidID int64, lineKey string, in Input) (*Entry, bool, error) {
	keys, err := s.repo.AddonLineKeys(ctx, raidID)
	if err != nil {
		return nil, false, err
	}
	if id, ok := keys[lineKey]; ok {
		entry, err := s.repo.get(ctx, id)
		return entry, false, err
	}
	key := lineKey
	entry, err := s.create(ctx, actorID, raidID, in, "addon", &key)
	return entry, err == nil, err
}

func (s *Service) create(ctx context.Context, actorID, raidID int64, in Input, source string, lineKey *string) (*Entry, error) {
	rd, err := s.raids.Get(ctx, raidID)
	if err != nil {
		return nil, err
	}
	if rd.Status == validation.RaidStatusCancelled {
		return nil, ErrRaidState
	}
	reason := strings.TrimSpace(in.Reason)
	if rd.ArchivedAt != nil && reason == "" {
		return nil, ErrReasonRequired
	}
	in.ItemID, in.ItemName = strings.TrimSpace(in.ItemID), strings.TrimSpace(in.ItemName)
	if !in.Outcome.Valid() || (in.ItemName == "" && in.ItemID == "") {
		return nil, ErrInvalid
	}
	entry := &Entry{RaidID: raidID, ItemID: in.ItemID, ItemName: in.ItemName, Outcome: in.Outcome,
		HolderName: strings.TrimSpace(in.HolderName), Notes: strings.TrimSpace(in.Notes), Source: source,
		AddonLineKey: lineKey, CreatedByPlayerID: actorID}
	if err := s.resolveCatalog(ctx, rd, in.CatalogBossID, entry); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	entry.CreatedAt, entry.UpdatedAt = now, now
	if err := s.repo.insert(ctx, entry, actorID, reason); err != nil {
		return nil, err
	}
	return entry, nil
}

// resolveCatalog checks the boss against the raid's instance and fills the
// boss and item names from the catalog where it knows them.
func (s *Service) resolveCatalog(ctx context.Context, rd *raid.Raid, bossID string, entry *Entry) error {
	catalog, err := s.catalog.List(ctx)
	if err != nil {
		return err
	}
	if entry.ItemID != "" {
		for _, item := range catalog.Items {
			if item.ItemID == entry.ItemID {
				entry.ItemName = item.Name
				break
			}
		}
		if entry.ItemName == "" {
			entry.ItemName = "Item " + entry.ItemID
		}
	}
	bossID = strings.TrimSpace(bossID)
	if bossID == "" {
		return nil
	}
	for _, boss := range catalog.Bosses {
		if boss.ID == bossID && rd.CatalogInstanceID != nil && boss.InstanceID == *rd.CatalogInstanceID {
			id := boss.ID
			entry.CatalogBossID, entry.Boss = &id, boss.Name
			return nil
		}
	}
	return ErrBoss
}

// Remove archives a wrongly recorded entry; history keeps it (ADR 007).
func (s *Service) Remove(ctx context.Context, actorID, id int64, reason string) error {
	entry, err := s.repo.get(ctx, id)
	if err != nil {
		return err
	}
	if entry.ArchivedAt != nil {
		return ErrAlreadyRemoved
	}
	rd, err := s.raids.Get(ctx, entry.RaidID)
	if err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	if rd.ArchivedAt != nil && reason == "" {
		return ErrReasonRequired
	}
	before := *entry
	now := time.Now().UTC()
	entry.ArchivedAt, entry.ArchivedByPlayerID, entry.ArchiveReason, entry.UpdatedAt = &now, &actorID, reason, now
	return s.repo.archive(ctx, &before, entry, actorID, reason)
}

// AddonLineKeys maps the addon export lines already recorded for a raid to
// their entry, so an import preview can mark them.
func (s *Service) AddonLineKeys(ctx context.Context, raidID int64) (map[string]int64, error) {
	return s.repo.AddonLineKeys(ctx, raidID)
}
