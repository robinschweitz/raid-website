package lootcatalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Service reads the authoritative bundled catalog and initializes its FK mirror.
type Service struct {
	repo *Repo
}

// NewService constructs the catalog domain service.
func NewService(repo *Repo) *Service {
	return &Service{repo: repo}
}

// Canonical validates a normalized dataset and hashes a deterministic sorted
// representation. The exporter uses it; the server never uploads datasets.
func Canonical(input Dataset) (Dataset, Report) {
	canonical := input
	canonical.Notices = append([]string{}, input.Notices...)
	canonical.Instances = append([]Instance{}, input.Instances...)
	canonical.Bosses = append([]Boss{}, input.Bosses...)
	canonical.Items = make([]Item, len(input.Items))
	for i, item := range input.Items {
		canonical.Items[i] = cloneItem(item)
	}
	canonical.Drops = append([]Drop{}, input.Drops...)

	report := Report{Source: canonical.Source, Notices: canonical.Notices, Errors: []string{}}
	for _, value := range []string{canonical.Source.ID, canonical.Source.Revision, canonical.Source.URL, canonical.Source.License, canonical.Source.Attribution} {
		if strings.TrimSpace(value) == "" {
			report.Errors = append(report.Errors, "source: ID, Revision, URL, Lizenz und Attribution erforderlich")
			break
		}
	}
	if len(canonical.Instances) == 0 || len(canonical.Bosses) == 0 || len(canonical.Items) == 0 || len(canonical.Drops) == 0 {
		report.Errors = append(report.Errors, "Katalog muss Instanzen, Bosse, Gegenstaende und Drops enthalten")
	}

	validID := func(id string) bool {
		number, err := strconv.ParseUint(id, 10, 32)
		return err == nil && number > 0 && strconv.FormatUint(number, 10) == id
	}
	instances := map[string]bool{}
	bosses := map[string]bool{}
	items := map[string]bool{}
	check := func(kind, id, name string, seen map[string]bool) {
		if !validID(id) || strings.TrimSpace(name) == "" {
			report.Errors = append(report.Errors, fmt.Sprintf("%s %q: ungueltige ID oder leerer Name", kind, id))
		}
		if seen[id] {
			report.Errors = append(report.Errors, fmt.Sprintf("%s %s: doppelte ID", kind, id))
		}
		seen[id] = true
	}
	for _, instance := range canonical.Instances {
		check("instance", instance.ID, instance.Name, instances)
	}
	for _, boss := range canonical.Bosses {
		if strings.HasPrefix(boss.ID, "encounter:") {
			if !regexp.MustCompile(`^encounter:`+regexp.QuoteMeta(boss.InstanceID)+`:[a-z][a-z0-9-]*$`).MatchString(boss.ID) || strings.TrimSpace(boss.Name) == "" || bosses[boss.ID] {
				report.Errors = append(report.Errors, "invalid or duplicate encounter "+boss.ID)
			}
			bosses[boss.ID] = true
		} else {
			check("boss", boss.ID, boss.Name, bosses)
		}
		if !instances[boss.InstanceID] {
			report.Errors = append(report.Errors, "boss "+boss.ID+": unbekannte instance_id")
		}
	}
	for _, item := range canonical.Items {
		check("item", item.ItemID, item.Name, items)
		if item.Binding != nil && *item.Binding < 0 || item.Quality != nil && *item.Quality < 0 {
			report.Errors = append(report.Errors, "item "+item.ItemID+": negative Metadaten")
		}
	}
	seenDrops := map[string]bool{}
	for _, drop := range canonical.Drops {
		if !bosses[drop.BossID] || !items[drop.ItemID] {
			report.Errors = append(report.Errors, "drop: unbekannter Boss oder Gegenstand")
		}
		if drop.RaidSize != 10 && drop.RaidSize != 25 || strings.TrimSpace(drop.Difficulty) == "" || drop.LootMode < 1 || drop.LootMode > 65535 {
			report.Errors = append(report.Errors, "drop: ungueltige Variante oder LootMode")
		}
		key := dropKey(drop)
		if seenDrops[key] {
			report.Errors = append(report.Errors, "drop: doppelte Beziehung "+key)
		}
		seenDrops[key] = true
	}
	sort.Slice(canonical.Instances, func(i, j int) bool { return canonical.Instances[i].ID < canonical.Instances[j].ID })
	sort.Slice(canonical.Bosses, func(i, j int) bool { return canonical.Bosses[i].ID < canonical.Bosses[j].ID })
	sort.Slice(canonical.Items, func(i, j int) bool { return canonical.Items[i].ItemID < canonical.Items[j].ItemID })
	sort.Slice(canonical.Drops, func(i, j int) bool { return dropKey(canonical.Drops[i]) < dropKey(canonical.Drops[j]) })
	encoded, err := json.Marshal(canonical)
	if err != nil {
		report.Errors = append(report.Errors, "Katalog kann nicht serialisiert werden")
		return canonical, report
	}
	hash := sha256.Sum256(encoded)
	report.RawHash = hex.EncodeToString(hash[:])
	return canonical, report
}

func dropKey(drop Drop) string {
	encoded, _ := json.Marshal(drop)
	return string(encoded)
}

// EnsureBundled transactionally mirrors the compiled catalog without changing
// guild history, and reports every changed fact.
func (s *Service) EnsureBundled(ctx context.Context) (InitializationReport, error) {
	return s.repo.EnsureBundled(ctx)
}

// List returns the complete bundled catalog without network or database access.
func (s *Service) List(ctx context.Context) (Catalog, error) {
	return s.repo.List(ctx)
}

// Instances lists the local game instances.
func (s *Service) Instances(ctx context.Context) ([]Instance, error) {
	catalog, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	return catalog.Instances, nil
}

// Bosses lists bosses, optionally restricted to one instance, in the
// Admin-set order (ADR 088).
func (s *Service) Bosses(ctx context.Context, instanceID string) ([]Boss, error) {
	catalog, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	bosses := []Boss{}
	for _, boss := range catalog.Bosses {
		if instanceID == "" || boss.InstanceID == instanceID {
			bosses = append(bosses, boss)
		}
	}
	positions, err := s.repo.bossPositions(ctx)
	if err != nil {
		return nil, err
	}
	sortBosses(bosses, positions)
	return bosses, nil
}

// BossesByItem returns the distinct bosses that drop each catalog item, keyed by item ID
// and sorted by boss ID. More than one boss means the item cannot identify one source,
// so callers must not derive a boss from it.
func (s *Service) BossesByItem(ctx context.Context) (map[string][]Boss, error) {
	catalog, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	known := make(map[string]Boss, len(catalog.Bosses))
	for _, boss := range catalog.Bosses {
		known[boss.ID] = boss
	}
	found := map[string]map[string]Boss{}
	for _, drop := range catalog.Drops {
		boss, ok := known[drop.BossID]
		if !ok {
			continue
		}
		if found[drop.ItemID] == nil {
			found[drop.ItemID] = map[string]Boss{}
		}
		found[drop.ItemID][boss.ID] = boss
	}
	out := make(map[string][]Boss, len(found))
	for itemID, bosses := range found {
		list := make([]Boss, 0, len(bosses))
		for _, boss := range bosses {
			list = append(list, boss)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
		out[itemID] = list
	}
	return out, nil
}

// Items merges every source relationship of one boss, raid size, and gameplay
// mode into a single selection and keeps the raw paths for provenance.
func (s *Service) Items(ctx context.Context, filter Filter) ([]ItemDrop, error) {
	if filter.RaidSize != 0 && filter.RaidSize != 10 && filter.RaidSize != 25 {
		return []ItemDrop{}, ErrInvalid
	}
	if filter.Mode != "" && !validMode(filter.Mode) {
		return []ItemDrop{}, ErrInvalid
	}
	catalog, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	items := map[string]Item{}
	for _, item := range catalog.Items {
		items[item.ItemID] = item
	}
	result := []ItemDrop{}
	indexes := map[string]int{}
	for _, drop := range catalog.Drops {
		if filter.BossID != "" && drop.BossID != filter.BossID {
			continue
		}
		if filter.RaidSize != 0 && drop.RaidSize != filter.RaidSize {
			continue
		}
		mode, conditional, err := sourceGameMode(drop)
		if err != nil {
			return nil, err
		}
		if filter.Mode != "" && mode != filter.Mode {
			continue
		}
		variant := SourceVariant{Difficulty: drop.Difficulty, LootMode: drop.LootMode}
		key := fmt.Sprintf("%s/%s/%d/%s", drop.BossID, drop.ItemID, drop.RaidSize, mode)
		if index, merged := indexes[key]; merged {
			result[index].SourceVariants = append(result[index].SourceVariants, variant)
			result[index].Conditional = result[index].Conditional || conditional
			continue
		}
		indexes[key] = len(result)
		result = append(result, ItemDrop{
			Item:           cloneItem(items[drop.ItemID]),
			BossID:         drop.BossID,
			RaidSize:       drop.RaidSize,
			Mode:           mode,
			Conditional:    conditional,
			SourceVariants: []SourceVariant{variant},
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].RaidSize != result[j].RaidSize {
			return result[i].RaidSize < result[j].RaidSize
		}
		if result[i].Mode != result[j].Mode {
			return result[i].Mode < result[j].Mode
		}
		return result[i].ItemID < result[j].ItemID
	})
	return result, nil
}

// ValidateAssociation accepts only instances and sizes evidenced by the bundled
// drops. Nil values keep existing free-text raids unassociated.
func (s *Service) ValidateAssociation(ctx context.Context, instanceID *string, raidSize *int) error {
	if instanceID == nil {
		if raidSize != nil {
			return ErrInvalid
		}
		return nil
	}
	catalog, err := s.List(ctx)
	if err != nil {
		return err
	}
	instance := Instance{}
	for _, candidate := range catalog.Instances {
		if candidate.ID == *instanceID {
			instance = candidate
		}
	}
	if instance.ID == "" {
		return ErrInvalid
	}
	if raidSize == nil {
		return nil
	}
	bosses := map[string]bool{}
	for _, boss := range catalog.Bosses {
		if boss.InstanceID == instance.ID {
			bosses[boss.ID] = true
		}
	}
	for _, drop := range catalog.Drops {
		if bosses[drop.BossID] && drop.RaidSize == *raidSize {
			return nil
		}
	}
	return ErrInvalid
}

// ValidateDrop resolves canonical names only for a drop the bundled source
// evidences for that instance and raid size.
func (s *Service) ValidateDrop(ctx context.Context, instanceID string, raidSize *int, bossID *string, itemID string) (Boss, Item, error) {
	if bossID == nil || itemID == "" {
		return Boss{}, Item{}, ErrInvalid
	}
	catalog, err := s.List(ctx)
	if err != nil {
		return Boss{}, Item{}, err
	}
	boss := Boss{}
	for _, candidate := range catalog.Bosses {
		if candidate.ID == *bossID && candidate.InstanceID == instanceID {
			boss = candidate
			break
		}
	}
	if boss.ID == "" {
		return Boss{}, Item{}, ErrInvalid
	}
	for _, drop := range catalog.Drops {
		if drop.BossID != boss.ID || drop.ItemID != itemID {
			continue
		}
		if raidSize != nil && drop.RaidSize != *raidSize {
			continue
		}
		for _, item := range catalog.Items {
			if item.ItemID == itemID {
				return boss, cloneItem(item), nil
			}
		}
	}
	return Boss{}, Item{}, ErrInvalid
}
