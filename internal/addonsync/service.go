package addonsync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/lootcatalog"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/raidcorrection"
	"raid-website/internal/validation"
)

var (
	// ErrRaidNotFound means the raid in the URL does not exist.
	ErrRaidNotFound = errors.New("raid not found")
	// ErrRaidState means addon exports can only be imported into planned or open
	// raids that are not archived; archived raids use ordinary corrections.
	ErrRaidState = errors.New("raid does not accept addon imports")
)

// importNote marks awards created from an addon export.
const importNote = "Aus RaidSync-Addon importiert"

// encounterNPCs maps the creatures that die in multi-creature encounters to the
// catalog's encounter keys. Single-creature bosses use their creature ID as key.
var encounterNPCs = map[string]string{
	"36939": "encounter:631:gunship",              // High Overlord Saurfang (Alliance side)
	"36948": "encounter:631:gunship",              // Muradin Bronzebeard (Horde side)
	"37970": "encounter:631:blood-prince-council", // Prince Valanar
	"37972": "encounter:631:blood-prince-council", // Prince Keleseth
	"37973": "encounter:631:blood-prince-council", // Prince Taldaram
}

// Service builds invite exports and imports recorded loot.
type Service struct {
	raids       *raid.Repo
	attendance  *attendance.Repo
	characters  *character.Repo
	players     *player.Repo
	catalog     *lootcatalog.Service
	loot        *loot.Repo
	points      *enhancepoints.Repo
	corrections *raidcorrection.Service
}

// NewService wires the repositories the addon exchange reads and the
// correction service that performs the actual loot writes.
func NewService(raids *raid.Repo, attendanceRepo *attendance.Repo, characters *character.Repo, players *player.Repo, catalog *lootcatalog.Service, lootRepo *loot.Repo, points *enhancepoints.Repo, corrections *raidcorrection.Service) *Service {
	return &Service{raids: raids, attendance: attendanceRepo, characters: characters, players: players, catalog: catalog, loot: lootRepo, points: points, corrections: corrections}
}

// InviteExport renders the raid roster for the addon. Invites use the live
// character name, because the roster snapshot keeps the name from accept time
// and a renamed character could not be invited under it.
func (s *Service) InviteExport(ctx context.Context, raidID int64, includeBench bool) (string, error) {
	rd, err := s.getRaid(ctx, raidID)
	if err != nil {
		return "", err
	}
	rows, err := s.attendance.ListByRaid(ctx, raidID)
	if err != nil {
		return "", fmt.Errorf("list attendance for raid %d: %w", raidID, err)
	}
	members := make([]InviteMember, 0, len(rows))
	for _, row := range rows {
		if row.IsBench && !includeBench {
			continue
		}
		name, err := s.currentCharacterName(ctx, row)
		if err != nil {
			return "", err
		}
		members = append(members, InviteMember{Name: name, Role: row.Role, Class: row.Class, IsBench: row.IsBench})
	}
	return BuildInviteExport(rd.ID, rd.Name, rd.RaidSize, members), nil
}

// LootPreview is the dry run of a loot import. It never writes.
type LootPreview struct {
	RaidID           int64             `json:"raid_id"`
	HeaderRaidID     int64             `json:"header_raid_id"`
	Recorder         string            `json:"recorder"`
	Rows             []LootPreviewRow  `json:"rows"`
	RecipientOptions []RecipientOption `json:"recipient_options"`
	Warnings         []string          `json:"warnings"`
}

// RecipientOption is a roster character staff may pick as the real winner,
// e.g. when another player passed a unique item on by trade.
type RecipientOption struct {
	CharacterID   int64  `json:"character_id"`
	CharacterName string `json:"character_name"`
}

// LootPreviewRow is one recorded drop with everything the website resolved for
// it. IsImportable rows can be applied once a boss is chosen.
type LootPreviewRow struct {
	LineKey       string             `json:"line_key"`
	LineNumber    int                `json:"line_number"`
	LootedAt      *time.Time         `json:"looted_at"`
	NPCID         string             `json:"npc_id"`
	ItemID        string             `json:"item_id"`
	ItemName      string             `json:"item_name"`
	RecipientName string             `json:"recipient_name"`
	State         LootState          `json:"state"`
	CharacterID   *int64             `json:"character_id"`
	CharacterName string             `json:"character_name"`
	CatalogBossID *string            `json:"catalog_boss_id"`
	BossOptions   []lootcatalog.Boss `json:"boss_options"`
	AwardMethod   loot.Method        `json:"award_method"`
	// Roll details of RS2 exports; empty for RS1.
	Roll              *int     `json:"roll"`
	Spec              string   `json:"spec"`
	RollRange         string   `json:"roll_range"`
	WinnerName        string   `json:"winner_name"`
	EnhancedPoints    int      `json:"enhanced_points"`
	IsImportable      bool     `json:"is_importable"`
	IsAlreadyImported bool     `json:"is_already_imported"`
	Warnings          []string `json:"warnings"`
	Errors            []string `json:"errors"`
}

// PreviewLootImport resolves boss, item and recipient for every recorded drop.
func (s *Service) PreviewLootImport(ctx context.Context, raidID int64, text string) (*LootPreview, error) {
	rd, err := s.importableRaid(ctx, raidID)
	if err != nil {
		return nil, err
	}
	export, err := ParseLootExport(text)
	if err != nil {
		return nil, err
	}
	resolver, err := s.newResolver(ctx, rd)
	if err != nil {
		return nil, err
	}
	preview := &LootPreview{RaidID: rd.ID, HeaderRaidID: export.RaidID, Recorder: export.Recorder, Rows: []LootPreviewRow{}, RecipientOptions: resolver.recipientOptions(), Warnings: []string{}}
	if export.RaidID != rd.ID {
		preview.Warnings = append(preview.Warnings, fmt.Sprintf("Der Export wurde für Raid #%d aufgezeichnet, nicht für diesen Raid (#%d).", export.RaidID, rd.ID))
	}
	keys := lineKeys(rd.ID, export)
	for i, line := range export.Lines {
		preview.Rows = append(preview.Rows, resolver.resolve(ctx, keys[i], line))
	}
	return preview, nil
}

// LootSelection is the staff decision for one previewed line.
type LootSelection struct {
	LineKey       string      `json:"line_key"`
	CatalogBossID string      `json:"catalog_boss_id"`
	CharacterID   int64       `json:"character_id"`
	AwardMethod   loot.Method `json:"award_method"`
}

// Import result statuses.
const (
	ResultCreated         = "created"
	ResultAlreadyImported = "already_imported"
	ResultFailed          = "failed"
)

// LootImportResult reports what happened to one selected line.
type LootImportResult struct {
	LineKey       string `json:"line_key"`
	LineNumber    int    `json:"line_number"`
	Status        string `json:"status"`
	AwardID       *int64 `json:"award_id"`
	CharacterName string `json:"character_name"`
	Code          string `json:"code"`
	Message       string `json:"message"`
}

// ApplyLootImport records the selected lines through the ordinary loot path,
// so catalog validation, roster checks, item locks and Enhanced costs apply
// exactly as for a manual entry. Every line is its own idempotent write and
// the result reports each one; nothing is overwritten.
func (s *Service) ApplyLootImport(ctx context.Context, raidID, actorID int64, text string, selections []LootSelection) ([]LootImportResult, error) {
	rd, err := s.importableRaid(ctx, raidID)
	if err != nil {
		return nil, err
	}
	export, err := ParseLootExport(text)
	if err != nil {
		return nil, err
	}
	keys := lineKeys(rd.ID, export)
	lines := make(map[string]LootLine, len(keys))
	for i, key := range keys {
		lines[key] = export.Lines[i]
	}
	imported, err := s.importedLineKeys(ctx, rd.ID)
	if err != nil {
		return nil, err
	}
	results := make([]LootImportResult, 0, len(selections))
	for _, selection := range selections {
		line, ok := lines[selection.LineKey]
		result := LootImportResult{LineKey: selection.LineKey, LineNumber: line.LineNumber}
		switch {
		case !ok:
			result.Status, result.Code, result.Message = ResultFailed, "unknown_line", "Zeile ist nicht im Export enthalten"
		case line.Error != "":
			result.Status, result.Code, result.Message = ResultFailed, "invalid_line", line.Error
		case !line.State.Importable():
			result.Status, result.Code, result.Message = ResultFailed, "not_importable", "Gelöschte oder verwahrte Zeilen werden nicht übernommen"
		case hasAward(imported, selection.LineKey):
			award := imported[selection.LineKey]
			result.Status, result.AwardID = ResultAlreadyImported, &award.ID
		default:
			award, err := s.importLine(ctx, rd, actorID, selection, line)
			switch {
			case errors.Is(err, loot.ErrAlreadyRecorded):
				// Another staff member imported the line after the pre-read.
				existing, err := s.loot.GetByRaidRequest(ctx, rd.ID, selection.LineKey)
				if err != nil {
					return nil, fmt.Errorf("load imported line %s: %w", selection.LineKey, err)
				}
				result.Status, result.AwardID = ResultAlreadyImported, &existing.ID
				imported[selection.LineKey] = *existing
			case err != nil:
				result.Status = ResultFailed
				result.Code, result.Message = importErrorMessage(err)
			default:
				result.Status, result.AwardID = ResultCreated, &award.ID
				// An active item lock may have moved the award to the lock owner.
				if award.AwardedToCharacterID != nil {
					if c, err := s.characters.Get(ctx, *award.AwardedToCharacterID); err == nil {
						result.CharacterName = c.Name
					}
				}
				imported[selection.LineKey] = *award
			}
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *Service) importLine(ctx context.Context, rd *raid.Raid, actorID int64, selection LootSelection, line LootLine) (*loot.Award, error) {
	if strings.TrimSpace(selection.CatalogBossID) == "" {
		return nil, lootcatalog.ErrInvalid
	}
	rows, err := s.attendance.ListByRaid(ctx, rd.ID)
	if err != nil {
		return nil, fmt.Errorf("list attendance for raid %d: %w", rd.ID, err)
	}
	var recipient *attendance.Attendance
	for i := range rows {
		if rows[i].CharacterID == selection.CharacterID {
			recipient = &rows[i]
			break
		}
	}
	if recipient == nil {
		return nil, loot.ErrRoster
	}
	bossID := selection.CatalogBossID
	characterID := recipient.CharacterID
	requestID := selection.LineKey
	now := time.Now().UTC()
	award := &loot.Award{
		RaidID:               rd.ID,
		CatalogBossID:        &bossID,
		ItemID:               line.ItemID,
		AwardedToPlayerID:    recipient.PlayerID,
		AwardedToCharacterID: &characterID,
		AwardedByPlayerID:    actorID,
		Source:               "manual",
		AwardMethod:          selection.AwardMethod,
		RequestID:            &requestID,
		Notes:                importNote,
		AwardedAt:            now,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := s.corrections.SaveLoot(ctx, award, actorID, ""); err != nil {
		return nil, err
	}
	return award, nil
}

// importErrorMessage turns a failed write into a stable code and a German message.
func importErrorMessage(err error) (string, string) {
	switch {
	case errors.Is(err, lootcatalog.ErrInvalid):
		return "catalog_item_invalid", "Boss und Gegenstand passen nicht zur Raidinstanz und Raidgröße"
	case errors.Is(err, loot.ErrRoster):
		return "recipient_not_in_roster", "Empfänger steht nicht in der Aufstellung"
	case errors.Is(err, loot.ErrInvalidMethod):
		return "invalid_award_method", "Vergabeart muss MAIN, SECOND, ENHANCED oder BOE sein"
	case errors.Is(err, enhancepoints.ErrInsufficientBalance):
		return "insufficient_balance", "Spieler hat keine EP für Enhanced-Loot"
	case errors.Is(err, loot.ErrLockConflict):
		return "item_lock_conflict", "Mehrere aktive Item-Locks passen auf dieses Item"
	case errors.Is(err, loot.ErrRequestConflict), errors.Is(err, loot.ErrAlreadyRecorded):
		return "request_conflict", "Diese Zeile wurde bereits mit anderen Angaben importiert"
	case errors.Is(err, loot.ErrRaidState), errors.Is(err, raidcorrection.ErrRaidState):
		return "raid_state", "Raid erlaubt diese Änderung nicht"
	default:
		return "save_failed", "Loot konnte nicht gespeichert werden"
	}
}

func (s *Service) getRaid(ctx context.Context, raidID int64) (*raid.Raid, error) {
	rd, err := s.raids.Get(ctx, raidID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRaidNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load raid %d: %w", raidID, err)
	}
	return rd, nil
}

// openRaid loads a raid that still accepts addon imports: planned or open and
// not archived.
func (s *Service) openRaid(ctx context.Context, raidID int64) (*raid.Raid, error) {
	rd, err := s.getRaid(ctx, raidID)
	if err != nil {
		return nil, err
	}
	if rd.ArchivedAt != nil || (rd.Status != validation.RaidStatusPlanned && rd.Status != validation.RaidStatusOpen) {
		return nil, ErrRaidState
	}
	return rd, nil
}

// importableRaid is an open raid with the catalog context loot validation needs.
func (s *Service) importableRaid(ctx context.Context, raidID int64) (*raid.Raid, error) {
	rd, err := s.openRaid(ctx, raidID)
	if err != nil {
		return nil, err
	}
	if rd.CatalogInstanceID == nil || rd.RaidSize == nil {
		return nil, loot.ErrCatalogContextRequired
	}
	return rd, nil
}

func (s *Service) currentCharacterName(ctx context.Context, row attendance.Attendance) (string, error) {
	c, err := s.characters.Get(ctx, row.CharacterID)
	if errors.Is(err, sql.ErrNoRows) {
		return row.CharacterName, nil
	}
	if err != nil {
		return "", fmt.Errorf("load character %d: %w", row.CharacterID, err)
	}
	return c.Name, nil
}

// importedLineKeys finds awards of this raid created from earlier imports,
// whoever imported them, so a second import by another staff member is a no-op.
func (s *Service) importedLineKeys(ctx context.Context, raidID int64) (map[string]loot.Award, error) {
	awards, err := s.loot.ListByRaid(ctx, raidID)
	if err != nil {
		return nil, fmt.Errorf("list loot for raid %d: %w", raidID, err)
	}
	keys := map[string]loot.Award{}
	for _, award := range awards {
		if award.RequestID != nil && strings.HasPrefix(*award.RequestID, "raidsync-") {
			keys[*award.RequestID] = award
		}
	}
	return keys, nil
}

// resolver holds everything a preview needs about one raid.
type resolver struct {
	raid         *raid.Raid
	roster       []rosterEntry
	bossesByItem map[string][]lootcatalog.Boss
	locks        []enhancepoints.ItemLock
	imported     map[string]loot.Award
	service      *Service
	playerNames  map[int64]string
}

type rosterEntry struct {
	attendance  attendance.Attendance
	currentName string
}

func (s *Service) newResolver(ctx context.Context, rd *raid.Raid) (*resolver, error) {
	roster, err := s.rosterEntries(ctx, rd.ID)
	if err != nil {
		return nil, err
	}
	bossesByItem, err := s.catalog.BossesByItem(ctx)
	if err != nil {
		return nil, fmt.Errorf("load catalog bosses: %w", err)
	}
	locks, err := s.points.ListActiveLocksByRaid(ctx, rd.ID)
	if err != nil {
		return nil, fmt.Errorf("list active locks for raid %d: %w", rd.ID, err)
	}
	imported, err := s.importedLineKeys(ctx, rd.ID)
	if err != nil {
		return nil, err
	}
	return &resolver{raid: rd, roster: roster, bossesByItem: bossesByItem, locks: locks, imported: imported, service: s, playerNames: map[int64]string{}}, nil
}

func (r *resolver) resolve(ctx context.Context, key string, line LootLine) LootPreviewRow {
	row := LootPreviewRow{LineKey: key, LineNumber: line.LineNumber, NPCID: line.NPCID, ItemID: line.ItemID, RecipientName: line.RecipientName, State: line.State, BossOptions: []lootcatalog.Boss{}, Warnings: []string{}, Errors: []string{}}
	if line.Error != "" {
		row.Errors = append(row.Errors, fmt.Sprintf("Zeile %d: %s", line.LineNumber, line.Error))
		return row
	}
	if line.Roll > 0 {
		roll := line.Roll
		row.Roll = &roll
	}
	if line.RollLow > 0 {
		row.RollRange = fmt.Sprintf("%d-%d", line.RollLow, line.RollHigh)
	}
	row.Spec, row.WinnerName, row.EnhancedPoints = line.Spec, line.WinnerName, line.EnhancedPoints()
	lootedAt := line.LootedAt
	row.LootedAt = &lootedAt
	existing, isImported := r.imported[key]
	if isImported {
		row.IsAlreadyImported = true
		row.Warnings = append(row.Warnings, "Bereits importiert")
	}

	candidates := r.bossCandidates(ctx, line.ItemID)
	for _, candidate := range candidates {
		row.BossOptions = append(row.BossOptions, candidate.boss)
		if row.ItemName == "" {
			row.ItemName = candidate.item.Name
		}
	}
	switch {
	case len(candidates) == 0:
		row.Errors = append(row.Errors, "Gegenstand gehört nicht zur Beute dieser Raidinstanz und Raidgröße")
	case len(candidates) == 1:
		row.CatalogBossID = &candidates[0].boss.ID
	default:
		if isImported && existing.CatalogBossID != nil {
			// Show the boss staff chose when this line was imported.
			bossID := *existing.CatalogBossID
			row.CatalogBossID = &bossID
		} else if hinted := r.hintedBoss(line.NPCID, candidates); hinted != "" {
			row.CatalogBossID = &hinted
		} else {
			row.Warnings = append(row.Warnings, "Boss nicht eindeutig – bitte auswählen")
		}
	}

	row.AwardMethod = defaultMethod(line, row.ItemName)

	// The recipient is only a suggestion: staff may pick another roster
	// character, because a trade between two other players is invisible to
	// the recorder.
	// Once imported, only a recipient that differs from the stored award matters.
	suggested := line.RecipientName
	if line.State == LootHeld && line.WinnerName != "" {
		// The recorder still carries the item, but the roll already has a winner.
		suggested = line.WinnerName
	}
	entry := findRosterEntry(r.roster, suggested)
	switch {
	case !line.State.Importable() && !isImported:
	case isImported && entry != nil:
		characterID := entry.attendance.CharacterID
		row.CharacterID = &characterID
		row.CharacterName = entry.currentName
	case isImported:
	case line.RecipientName == "":
		row.Warnings = append(row.Warnings, "Nicht vergeben (auf der Leiche gelassen oder entzaubert) – bei Bedarf Empfänger wählen")
	case entry != nil:
		characterID := entry.attendance.CharacterID
		row.CharacterID = &characterID
		row.CharacterName = entry.currentName
	default:
		row.Warnings = append(row.Warnings, fmt.Sprintf("%s steht nicht in der Aufstellung – Empfänger wählen oder zuerst per Schnellaufnahme hinzufügen", suggested))
	}
	switch {
	case line.State == LootHeld && !isImported && line.WinnerName != "":
		row.Warnings = append(row.Warnings, fmt.Sprintf("Noch beim Plündermeister – gewonnen von %s, Handel steht aus", line.WinnerName))
	case line.State == LootHeld && !isImported:
		row.Warnings = append(row.Warnings, "Noch beim Plündermeister – Empfänger prüfen")
	case line.State == LootParked && !isImported:
		row.Warnings = append(row.Warnings, fmt.Sprintf("Wird von %s verwahrt und ist noch nicht verlost – wird nicht übernommen", line.RecipientName))
	case line.State == LootDeleted && isImported:
		row.Warnings = append(row.Warnings, "Im Addon gelöscht – bei Bedarf über die Loot-Korrektur zurückziehen")
	case line.State == LootDeleted:
		row.Warnings = append(row.Warnings, "Im Addon gelöscht – wird nicht übernommen")
	}
	if x := line.EnhancedPoints(); x > 0 && entry != nil && !isImported {
		if want := r.enhancedPoints(ctx, entry.attendance.PlayerID); want != x {
			row.Warnings = append(row.Warnings, fmt.Sprintf("Enhanced-Wurf mit %d EP, laut Konto sind es %d (Kontostand, höchstens %d) – abgebucht wird der Kontowert", x, want, maxEnhancedPoints))
		}
	}
	if isImported && existing.AwardedToCharacterID != nil && (row.CharacterID == nil || *existing.AwardedToCharacterID != *row.CharacterID) {
		row.Warnings = append(row.Warnings, fmt.Sprintf("Bereits importiert an %s, der Export nennt %s – bitte über die Loot-Korrektur ändern", r.characterName(ctx, *existing.AwardedToCharacterID), displayRecipient(line.RecipientName)))
	}

	for _, lock := range r.locks {
		if enhancepoints.ItemsMatch(lock.ItemID, lock.ItemName, line.ItemID, row.ItemName) {
			row.Warnings = append(row.Warnings, fmt.Sprintf("Aktiver Item-Lock von %s: der Gegenstand wird beim Übernehmen automatisch an diesen Spieler vergeben", r.playerName(ctx, lock.PlayerID)))
		}
	}

	row.IsImportable = len(row.Errors) == 0 && !row.IsAlreadyImported && line.State.Importable()
	return row
}

// Importable is false for drops that are not awards (yet): parked items wait
// for their roll and deleted ones were removed in the addon. Staff decide
// about those; the import never changes anything for them.
func (s LootState) Importable() bool {
	return s != LootParked && s != LootDeleted
}

// defaultMethod suggests the award method: an enhanced roll range means
// ENHANCED, tokens stay SECOND (ADR 028), and an RS2 off-spec roll SECOND.
func defaultMethod(line LootLine, itemName string) loot.Method {
	switch {
	case line.EnhancedPoints() > 0:
		return loot.Enhanced
	case loot.IsTokenAward(itemName), line.Spec == SpecOff:
		return loot.Second
	default:
		return loot.Main
	}
}

// enhancedPoints is X = min(balance, 50) for the player (ADR 023); -1 when the
// balance cannot be read, so the preview warns instead of guessing.
func (r *resolver) enhancedPoints(ctx context.Context, playerID int64) int {
	balance, err := r.service.points.Balance(ctx, playerID)
	if err != nil {
		return -1
	}
	return max(0, min(balance, maxEnhancedPoints))
}

func displayRecipient(name string) string {
	if name == "" {
		return "niemanden"
	}
	return name
}

type bossCandidate struct {
	boss lootcatalog.Boss
	item lootcatalog.Item
}

// bossCandidates lists the bosses of this raid's instance that drop the item
// at this raid size.
func (r *resolver) bossCandidates(ctx context.Context, itemID string) []bossCandidate {
	var candidates []bossCandidate
	for _, boss := range r.bossesByItem[itemID] {
		if boss.InstanceID != *r.raid.CatalogInstanceID {
			continue
		}
		bossID := boss.ID
		validBoss, item, err := r.service.catalog.ValidateDrop(ctx, *r.raid.CatalogInstanceID, r.raid.RaidSize, &bossID, itemID)
		if err != nil {
			continue
		}
		candidates = append(candidates, bossCandidate{boss: validBoss, item: item})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].boss.ID < candidates[j].boss.ID })
	return candidates
}

// hintedBoss picks the candidate matching the creature the addon saw die.
func (r *resolver) hintedBoss(npcID string, candidates []bossCandidate) string {
	if npcID == "" {
		return ""
	}
	bossID := npcID
	if encounterID, ok := encounterNPCs[npcID]; ok {
		bossID = encounterID
	}
	for _, candidate := range candidates {
		if candidate.boss.ID == bossID {
			return bossID
		}
	}
	return ""
}

// recipientOptions lists every roster character by current name.
func (r *resolver) recipientOptions() []RecipientOption {
	options := make([]RecipientOption, 0, len(r.roster))
	for _, entry := range r.roster {
		options = append(options, RecipientOption{CharacterID: entry.attendance.CharacterID, CharacterName: entry.currentName})
	}
	sort.Slice(options, func(i, j int) bool {
		return strings.ToLower(options[i].CharacterName) < strings.ToLower(options[j].CharacterName)
	})
	return options
}

func (r *resolver) characterName(ctx context.Context, characterID int64) string {
	for _, entry := range r.roster {
		if entry.attendance.CharacterID == characterID {
			return entry.currentName
		}
	}
	if c, err := r.service.characters.Get(ctx, characterID); err == nil {
		return c.Name
	}
	return fmt.Sprintf("Charakter #%d", characterID)
}

func (r *resolver) playerName(ctx context.Context, playerID int64) string {
	if name, ok := r.playerNames[playerID]; ok {
		return name
	}
	name := fmt.Sprintf("Spieler #%d", playerID)
	if p, err := r.service.players.Get(ctx, playerID); err == nil {
		name = p.Name
	}
	r.playerNames[playerID] = name
	return name
}

func hasAward(awards map[string]loot.Award, key string) bool {
	_, ok := awards[key]
	return ok
}
