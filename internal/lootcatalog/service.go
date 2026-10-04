package lootcatalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Service validates and merges only explicitly reviewed local datasets.
type Service struct{ repo *Repo }

// NewService constructs the catalog domain service.
func NewService(repo *Repo) *Service { return &Service{repo: repo} }

// Canonical validates the upload and hashes a deterministic sorted representation.
func Canonical(input Dataset) (Dataset, Report) {
	d := input
	d.Instances = append([]Instance{}, input.Instances...)
	d.Bosses = append([]Boss{}, input.Bosses...)
	d.Items = append([]Item{}, input.Items...)
	d.Drops = append([]Drop{}, input.Drops...)
	d.Notices = append([]string{}, input.Notices...)
	r := Report{Source: d.Source, Notices: d.Notices, Errors: []string{}, Conflicts: []string{}}
	for _, value := range []string{d.Source.ID, d.Source.Revision, d.Source.URL, d.Source.License, d.Source.Attribution} {
		if strings.TrimSpace(value) == "" {
			r.Errors = append(r.Errors, "source: ID, Revision, URL, Lizenz und Attribution erforderlich")
			break
		}
	}
	if len(d.Instances) == 0 || len(d.Bosses) == 0 || len(d.Items) == 0 || len(d.Drops) == 0 {
		r.Errors = append(r.Errors, "Katalog muss Instanzen, Bosse, Gegenstaende und Drops enthalten")
	}
	validID := func(id string) bool {
		n, err := strconv.ParseUint(id, 10, 32)
		return err == nil && n > 0 && strconv.FormatUint(n, 10) == id
	}
	instances, bosses, items := map[string]bool{}, map[string]bool{}, map[string]bool{}
	check := func(kind, id, name string, seen map[string]bool) {
		if !validID(id) || strings.TrimSpace(name) == "" {
			r.Errors = append(r.Errors, fmt.Sprintf("%s %q: ungueltige ID oder leerer Name", kind, id))
		}
		if seen[id] {
			r.Errors = append(r.Errors, fmt.Sprintf("%s %s: doppelte ID", kind, id))
		}
		seen[id] = true
	}
	for _, i := range d.Instances {
		check("instance", i.ID, i.Name, instances)
	}
	for _, b := range d.Bosses {
		check("boss", b.ID, b.Name, bosses)
		if !instances[b.InstanceID] {
			r.Errors = append(r.Errors, "boss "+b.ID+": unbekannte instance_id")
		}
	}
	for _, i := range d.Items {
		check("item", i.ItemID, i.Name, items)
		if i.Binding != nil && *i.Binding < 0 || i.Quality != nil && *i.Quality < 0 {
			r.Errors = append(r.Errors, "item "+i.ItemID+": negative Metadaten")
		}
	}
	seenDrops := map[string]bool{}
	for _, drop := range d.Drops {
		if !bosses[drop.BossID] || !items[drop.ItemID] {
			r.Errors = append(r.Errors, "drop: unbekannter Boss oder Gegenstand")
		}
		if drop.RaidSize != 10 && drop.RaidSize != 25 || strings.TrimSpace(drop.Difficulty) == "" || drop.LootMode < 1 || drop.LootMode > 65535 {
			r.Errors = append(r.Errors, "drop: ungueltige Variante oder LootMode")
		}
		key := dropKey(drop)
		if seenDrops[key] {
			r.Errors = append(r.Errors, "drop: doppelte Beziehung "+key)
		}
		seenDrops[key] = true
	}
	sort.Slice(d.Instances, func(i, j int) bool { return d.Instances[i].ID < d.Instances[j].ID })
	sort.Slice(d.Bosses, func(i, j int) bool { return d.Bosses[i].ID < d.Bosses[j].ID })
	sort.Slice(d.Items, func(i, j int) bool { return d.Items[i].ItemID < d.Items[j].ItemID })
	sort.Slice(d.Drops, func(i, j int) bool { return dropKey(d.Drops[i]) < dropKey(d.Drops[j]) })
	encoded, err := json.Marshal(d)
	if err != nil {
		r.Errors = append(r.Errors, "Katalog kann nicht serialisiert werden")
		return d, r
	}
	hash := sha256.Sum256(encoded)
	r.RawHash = hex.EncodeToString(hash[:])
	return d, r
}

func dropKey(d Drop) string { encoded, _ := json.Marshal(d); return string(encoded) }

// Preview writes only preview evidence when validation and database comparison succeed.
func (s *Service) Preview(ctx context.Context, input Dataset, suppliedHash string) (Report, error) {
	d, report := Canonical(input)
	if len(report.Errors) != 0 {
		return report, ErrInvalid
	}
	if suppliedHash != "" && suppliedHash != report.RawHash {
		return report, ErrHashMismatch
	}
	return s.repo.Import(ctx, d, report, false)
}

// Merge requires a matching successful preview and rechecks conflicts transactionally.
func (s *Service) Merge(ctx context.Context, input Dataset, reviewedHash string) (Report, error) {
	d, report := Canonical(input)
	if len(report.Errors) != 0 {
		return report, ErrInvalid
	}
	if reviewedHash == "" {
		return report, ErrPreviewRequired
	}
	if reviewedHash != report.RawHash {
		return report, ErrHashMismatch
	}
	return s.repo.Import(ctx, d, report, true)
}

// List returns the complete local catalog without network access.
func (s *Service) List(ctx context.Context) (Catalog, error) { return s.repo.List(ctx) }

// Instances lists local game instances.
func (s *Service) Instances(ctx context.Context) ([]Instance, error) {
	c, err := s.List(ctx)
	return c.Instances, err
}

// Bosses lists bosses, optionally restricted to an instance.
func (s *Service) Bosses(ctx context.Context, instanceID string) ([]Boss, error) {
	c, err := s.List(ctx)
	result := []Boss{}
	for _, b := range c.Bosses {
		if instanceID == "" || b.InstanceID == instanceID {
			result = append(result, b)
		}
	}
	return result, err
}

// Items returns item/drop pairs, preserving mixed hardmode metadata.
func (s *Service) Items(ctx context.Context, filter Filter) ([]ItemDrop, error) {
	if filter.RaidSize != 0 && filter.RaidSize != 10 && filter.RaidSize != 25 {
		return []ItemDrop{}, ErrInvalid
	}
	c, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	items := map[string]Item{}
	for _, i := range c.Items {
		items[i.ItemID] = i
	}
	result := []ItemDrop{}
	for _, d := range c.Drops {
		if filter.BossID != "" && d.BossID != filter.BossID || filter.RaidSize != 0 && d.RaidSize != filter.RaidSize || filter.Difficulty != "" && d.Difficulty != filter.Difficulty {
			continue
		}
		result = append(result, ItemDrop{Item: items[d.ItemID], BossID: d.BossID, RaidSize: d.RaidSize, Difficulty: d.Difficulty, LootMode: d.LootMode})
	}
	return result, nil
}

// ValidateAssociation accepts only sizes evidenced by drops for the selected instance.
// Nil values allow existing free-text raids to remain unassociated.
func (s *Service) ValidateAssociation(ctx context.Context, instanceID *string, raidSize *int) error {
	if instanceID == nil {
		if raidSize != nil {
			return ErrInvalid
		}
		return nil
	}
	c, err := s.List(ctx)
	if err != nil {
		return err
	}
	found := false
	bosses := map[string]bool{}
	for _, i := range c.Instances {
		if i.ID == *instanceID {
			found = true
		}
	}
	if !found {
		return ErrInvalid
	}
	if raidSize == nil {
		return nil
	}
	for _, b := range c.Bosses {
		if b.InstanceID == *instanceID {
			bosses[b.ID] = true
		}
	}
	for _, d := range c.Drops {
		if bosses[d.BossID] && d.RaidSize == *raidSize {
			return nil
		}
	}
	return ErrInvalid
}

// ValidateDrop resolves canonical names only for an evidenced local drop association.
func (s *Service) ValidateDrop(ctx context.Context, instanceID string, raidSize *int, bossID *string, itemID string) (Boss, Item, error) {
	if bossID == nil || itemID == "" {
		return Boss{}, Item{}, ErrInvalid
	}
	c, err := s.List(ctx)
	if err != nil {
		return Boss{}, Item{}, err
	}
	var boss Boss
	for _, b := range c.Bosses {
		if b.ID == *bossID && b.InstanceID == instanceID {
			boss = b
			break
		}
	}
	if boss.ID == "" {
		return Boss{}, Item{}, ErrInvalid
	}
	for _, d := range c.Drops {
		if d.BossID == boss.ID && d.ItemID == itemID && (raidSize == nil || d.RaidSize == *raidSize) {
			for _, item := range c.Items {
				if item.ItemID == itemID {
					return boss, item, nil
				}
			}
		}
	}
	return Boss{}, Item{}, ErrInvalid
}
